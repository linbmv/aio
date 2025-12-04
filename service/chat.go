package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"time"

	"github.com/atopos31/llmio/balancers"
	"github.com/atopos31/llmio/consts"
	"github.com/atopos31/llmio/models"
	"github.com/atopos31/llmio/providers"
	"github.com/atopos31/llmio/service/errorx"
	"github.com/atopos31/llmio/service/formatx"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

func BalanceChat(ctx context.Context, start time.Time, clientFormat string, before Before, providersWithMeta ProvidersWithMeta, reqMeta models.ReqMeta) (*http.Response, uint, string, error) {
	slog.Info("request", "model", before.Model, "stream", before.Stream, "tool_call", before.toolCall, "structured_output", before.structuredOutput, "image", before.image)

	providerMap := providersWithMeta.ProviderMap

	// 收集重试过程中的err日志
	retryLog := make(chan models.ChatLog, providersWithMeta.MaxRetry)
	defer close(retryLog)

	go RecordRetryLog(context.Background(), retryLog)

	// 选择负载均衡策略
	var balancer balancers.Balancer
	switch providersWithMeta.Strategy {
	case consts.BalancerLottery:
		balancer = balancers.NewLottery(providersWithMeta.WeightItems)
	case consts.BalancerRotor:
		balancer = balancers.NewRotor(providersWithMeta.WeightItems)
	default:
		balancer = balancers.NewLottery(providersWithMeta.WeightItems)
	}

	client := providers.GetClient(time.Second * time.Duration(providersWithMeta.TimeOut) / 3)

	// 请求体转换缓存
	reqCache := map[string][]byte{clientFormat: before.raw}
	getReqBody := func(providerType string) ([]byte, error) {
		if data, ok := reqCache[providerType]; ok {
			return data, nil
		}
		converted, err := formatx.ConvertRequest(before.raw, clientFormat, providerType)
		if err != nil {
			return nil, err
		}
		reqCache[providerType] = converted
		return converted, nil
	}

	timer := time.NewTimer(time.Second * time.Duration(providersWithMeta.TimeOut))
	defer timer.Stop()
	for retry := 0; retry < providersWithMeta.MaxRetry; retry++ {
		select {
		case <-ctx.Done():
			return nil, 0, "", ctx.Err()
		case <-timer.C:
			return nil, 0, "", errors.New("retry time out")
		default:
			// 加权负载均衡
			id, err := balancer.Pop()
			if err != nil {
				return nil, 0, "", err
			}

			modelWithProvider, ok := providersWithMeta.ModelWithProviderMap[id]
			if !ok {
				// 数据不一致，移除该模型避免下次重复命中
				balancer.Delete(id)
				continue
			}

			// 检查渠道是否冷却中
			if providers.IsChannelCoolingDown(modelWithProvider.ModelID, modelWithProvider.ProviderID) {
				slog.Info("channel is cooling down, skip", "model_id", modelWithProvider.ModelID, "provider_id", modelWithProvider.ProviderID)
				continue
			}

			provider := providerMap[modelWithProvider.ProviderID]

			chatModel, err := providers.New(provider.Type, provider.Config, provider.ID)
			if err != nil {
				return nil, 0, "", err
			}

			slog.Info("using provider", "provider", provider.Name, "model", modelWithProvider.ProviderModel)

			log := models.ChatLog{
				Name:          before.Model,
				ProviderModel: modelWithProvider.ProviderModel,
				ProviderName:  provider.Name,
				Status:        "success",
				Style:         clientFormat,
				UserAgent:     reqMeta.UserAgent,
				RemoteIP:      reqMeta.RemoteIP,
				ChatIO:        providersWithMeta.IOLog,
				Retry:         retry,
				ProxyTime:     time.Since(start),
			}
			// 根据请求原始请求头 是否透传请求头 自定义请求头 构建新的请求头
			withHeader := false
			if modelWithProvider.WithHeader != nil {
				withHeader = *modelWithProvider.WithHeader
			}
			header := buildHeaders(reqMeta.Header, withHeader, modelWithProvider.CustomerHeaders, before.Stream)

			reqStart := time.Now()
			trace := &httptrace.ClientTrace{
				GotFirstResponseByte: func() {
					fmt.Printf("响应时间: %v", time.Since(reqStart))
				},
			}

			reqBody, err := getReqBody(provider.Type)
			if err != nil {
				retryLog <- log.WithError(err)
				continue
			}

			// 为每个请求设置超时 context
			reqCtx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(providersWithMeta.TimeOut))

			req, usedKey, err := chatModel.BuildReq(httptrace.WithClientTrace(reqCtx, trace), header, modelWithProvider.ProviderModel, reqBody)
			if err != nil {
				cancel()
				retryLog <- log.WithError(err)
				continue
			}

			res, err := client.Do(req)
			if err != nil {
				cancel()
				retryLog <- log.WithError(err)
				continue
			}

			if res.StatusCode != http.StatusOK {
				byteBody, _ := io.ReadAll(res.Body)
				res.Body.Close()
				cancel()

				classifiedErr := errorx.ClassifyHTTPError(res.StatusCode, byteBody, res.Header)
				retryLog <- log.WithError(fmt.Errorf("status: %d, body: %s", res.StatusCode, string(byteBody)))

				switch classifiedErr.Level {
				case errorx.ErrorKey:
					if usedKey != "" {
						providers.MarkKeyFailure(provider.ID, usedKey, 0)
					}
					if classifiedErr.Code == "rate_limit_key" {
						balancer.Reduce(id)
					}
				case errorx.ErrorChannel:
					providers.MarkChannelFailure(modelWithProvider.ModelID, provider.ID, 2*time.Minute)
					balancer.Delete(id)
				case errorx.ErrorClient:
					return nil, 0, provider.Type, fmt.Errorf("client error: %s", string(byteBody))
				}
				continue
			}

			logId, err := SaveChatLog(ctx, log)
			if err != nil {
				res.Body.Close()
				cancel()
				return nil, 0, "", err
			}

			// 成功路径：保持 reqCtx 存活，让调用方读完 res.Body 后自然释放
			return res, logId, provider.Type, nil
		}
	}

	return nil, 0, "", errors.New("maximum retry attempts reached")
}

func RecordRetryLog(ctx context.Context, retryLog chan models.ChatLog) {
	for log := range retryLog {
		if _, err := SaveChatLog(ctx, log); err != nil {
			slog.Error("save chat log error", "error", err)
		}
	}
}

func RecordLog(ctx context.Context, reqStart time.Time, reader io.ReadCloser, processer Processer, logId uint, before Before, ioLog bool) {
	recordFunc := func() error {
		defer reader.Close()
		if ioLog {
			if err := gorm.G[models.ChatIO](models.DB).Create(ctx, &models.ChatIO{
				Input: string(before.raw),
				LogId: logId,
			}); err != nil {
				return err
			}
		}
		log, output, err := processer(ctx, reader, before.Stream, reqStart)
		if err != nil {
			return err
		}
		if _, err := gorm.G[models.ChatLog](models.DB).Where("id = ?", logId).Updates(ctx, *log); err != nil {
			return err
		}
		if ioLog {
			if _, err := gorm.G[models.ChatIO](models.DB).Where("log_id = ?", logId).Updates(ctx, models.ChatIO{OutputUnion: *output}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := recordFunc(); err != nil {
		slog.Error("record log error", "error", err)
	}
}

func SaveChatLog(ctx context.Context, log models.ChatLog) (uint, error) {
	if err := gorm.G[models.ChatLog](models.DB).Create(ctx, &log); err != nil {
		return 0, err
	}
	return log.ID, nil
}

func buildHeaders(source http.Header, withHeader bool, customHeaders map[string]string, stream bool) http.Header {
	header := http.Header{}
	if withHeader {
		header = source.Clone()
	}

	if stream {
		header.Set("X-Accel-Buffering", "no")
	}

	header.Del("Authorization")
	header.Del("X-Api-Key")

	for key, value := range customHeaders {
		header.Set(key, value)
	}

	return header
}

type ProvidersWithMeta struct {
	ModelWithProviderMap map[uint]models.ModelWithProvider
	WeightItems          map[uint]int
	ProviderMap          map[uint]models.Provider
	MaxRetry             int
	TimeOut              int
	IOLog                bool
	Strategy             string // 负载均衡策略
}

func ProvidersWithMetaBymodelsName(ctx context.Context, style string, before Before) (*ProvidersWithMeta, error) {
	model, err := gorm.G[models.Model](models.DB).Where("name = ?", before.Model).First(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if _, err := SaveChatLog(ctx, models.ChatLog{
				Name:   before.Model,
				Status: "error",
				Style:  style,
				Error:  err.Error(),
			}); err != nil {
				return nil, err
			}
			return nil, errors.New("not found model " + before.Model)
		}
		return nil, err
	}

	modelWithProviderChain := gorm.G[models.ModelWithProvider](models.DB).Where("model_id = ?", model.ID).Where("status = ?", true)

	if before.toolCall {
		modelWithProviderChain = modelWithProviderChain.Where("tool_call = ?", true)
	}

	if before.structuredOutput {
		modelWithProviderChain = modelWithProviderChain.Where("structured_output = ?", true)
	}

	if before.image {
		modelWithProviderChain = modelWithProviderChain.Where("image = ?", true)
	}

	modelWithProviders, err := modelWithProviderChain.Find(ctx)
	if err != nil {
		return nil, err
	}

	if len(modelWithProviders) == 0 {
		return nil, errors.New("not provider for model " + before.Model)
	}

	modelWithProviderMap := lo.KeyBy(modelWithProviders, func(mp models.ModelWithProvider) uint { return mp.ID })

	providers, err := gorm.G[models.Provider](models.DB).
		Where("id IN ?", lo.Map(modelWithProviders, func(mp models.ModelWithProvider, _ int) uint { return mp.ProviderID })).
		Find(ctx)
	if err != nil {
		return nil, err
	}

	// 过滤支持格式转换的 Provider
	filtered := make([]models.Provider, 0, len(providers))
	for _, p := range providers {
		if formatx.CanConvert(style, p.Type) {
			filtered = append(filtered, p)
		}
	}
	providerMap := lo.KeyBy(filtered, func(p models.Provider) uint { return p.ID })

	weightItems := make(map[uint]int)
	for _, mp := range modelWithProviders {
		if _, ok := providerMap[mp.ProviderID]; !ok {
			continue
		}
		weightItems[mp.ID] = mp.Weight
	}

	if len(weightItems) == 0 {
		return nil, errors.New("no convertible provider for model " + before.Model)
	}

	if model.IOLog == nil {
		model.IOLog = new(bool)
	}

	return &ProvidersWithMeta{
		ModelWithProviderMap: modelWithProviderMap,
		WeightItems:          weightItems,
		ProviderMap:          providerMap,
		MaxRetry:             model.MaxRetry,
		TimeOut:              model.TimeOut,
		IOLog:                *model.IOLog,
		Strategy:             model.Strategy,
	}, nil
}
