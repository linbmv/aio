package service

import (
	"context"
	"encoding/json"
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
	"github.com/atopos31/llmio/service/cooldown"
	"github.com/atopos31/llmio/service/keypool"
	"github.com/samber/lo"
	"gorm.io/gorm"
)

type streamContextKey struct{}

type streamContext struct {
	modelWithProvider *models.ModelWithProvider
	cooldownManager   *cooldown.Manager
	keyPool           *keypool.Pool
	keyID             uint
}

func withStreamContext(ctx context.Context, streamCtx *streamContext) context.Context {
	if streamCtx == nil {
		return ctx
	}
	return context.WithValue(ctx, streamContextKey{}, streamCtx)
}

func streamContextFrom(ctx context.Context) *streamContext {
	if ctx == nil {
		return nil
	}
	streamCtx, _ := ctx.Value(streamContextKey{}).(*streamContext)
	return streamCtx
}

func CopyStreamContext(ctx context.Context) context.Context {
	if streamCtx := streamContextFrom(ctx); streamCtx != nil {
		// 保留原始 context 的取消信号，只复制 stream context
		return withStreamContext(ctx, streamCtx)
	}
	return ctx
}

func BalanceChat(ctx context.Context, start time.Time, style string, before Before, providersWithMeta ProvidersWithMeta, reqMeta models.ReqMeta) (*http.Response, uint, error) {
	slog.Info("request", "model", before.Model, "stream", before.Stream, "tool_call", before.toolCall, "structured_output", before.structuredOutput, "image", before.image)

	providerMap := providersWithMeta.ProviderMap
	cooldownManager := cooldown.NewManager(models.DB)
	keyPool := keypool.NewPool(models.DB)

	// 鏀堕泦閲嶈瘯杩囩▼涓殑err鏃ュ織
	retryLog := make(chan models.ChatLog, providersWithMeta.MaxRetry)
	defer close(retryLog)

	go RecordRetryLog(context.Background(), retryLog)

	// 閫夋嫨璐熻浇鍧囪　绛栫暐
	var balancer balancers.Balancer
	switch providersWithMeta.Strategy {
	case consts.BalancerSmoothWeightedRR:
		balancer = balancers.NewSmoothWeightedRR(providersWithMeta.WeightItems)
	case consts.BalancerRotor:
		balancer = balancers.NewRotor(providersWithMeta.WeightItems)
	case consts.BalancerLottery, "":
		balancer = balancers.NewLottery(providersWithMeta.WeightItems)
	default:
		balancer = balancers.NewLottery(providersWithMeta.WeightItems)
	}

	// 璁剧疆璇锋眰瓒呮椂
	responseHeaderTimeout := time.Second * time.Duration(providersWithMeta.TimeOut)
	// 娴佸紡瓒呮椂鏃堕棿缂╃煭
	if before.Stream {
		responseHeaderTimeout = responseHeaderTimeout / 3
	}
	client := providers.GetClient(responseHeaderTimeout)

	authKeyID, _ := ctx.Value(consts.ContextKeyAuthKeyID).(uint)

	timer := time.NewTimer(time.Second * time.Duration(providersWithMeta.TimeOut))
	defer timer.Stop()

	retries := providersWithMeta.MaxRetry
	if retries <= 0 {
		retries = 1
	}
	activeProviders := len(providersWithMeta.WeightItems)
	if activeProviders == 0 {
		activeProviders = len(providersWithMeta.ModelWithProviderMap)
	}
	if activeProviders == 0 {
		return nil, 0, errors.New("no active providers")
	}
	cooldownSkipped := 0
	retry := 0
	for retry < retries {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-timer.C:
			return nil, 0, errors.New("retry time out")
		default:
			// 鍔犳潈璐熻浇鍧囪
			id, err := balancer.Pop()
			if err != nil {
				return nil, 0, err
			}

			modelWithProvider, ok := providersWithMeta.ModelWithProviderMap[id]
			if !ok {
				// 鏁版嵁涓嶄竴鑷达紝绉婚櫎璇ユā鍨嬮伩鍏嶄笅娆￠噸澶嶅懡涓?
				balancer.Delete(id)
				continue
			}
			if cooldownManager.InCooldown(modelWithProvider) {
				cooldownSkipped++
				balancer.Reduce(id)
				if cooldownSkipped >= activeProviders {
					return nil, 0, fmt.Errorf("all providers are in cooldown")
				}
				continue
			}
			cooldownSkipped = 0
			retry++

			provider := providerMap[modelWithProvider.ProviderID]

			chatModel, err := providers.New(style, provider.Config)
			if err != nil {
				return nil, 0, err
			}

			slog.Info("using provider", "provider", provider.Name, "model", modelWithProvider.ProviderModel)

			log := models.ChatLog{
				Name:          before.Model,
				ProviderModel: modelWithProvider.ProviderModel,
				ProviderName:  provider.Name,
				Status:        "success",
				Style:         style,
				UserAgent:     reqMeta.UserAgent,
				RemoteIP:      reqMeta.RemoteIP,
				AuthKeyID:     authKeyID,
				ProviderKeyID: 0, // 将在获取 key 后更新
				ChatIO:        providersWithMeta.IOLog,
				Retry:         retry,
				ProxyTime:     time.Since(start),
			}
			// 鏍规嵁璇锋眰鍘熷璇锋眰澶?鏄惁閫忎紶璇锋眰澶?鑷畾涔夎姹傚ご 鏋勫缓鏂扮殑璇锋眰澶?
			withHeader := false
			if modelWithProvider.WithHeader != nil {
				withHeader = *modelWithProvider.WithHeader
			}
			header := buildHeaders(reqMeta.Header, withHeader, modelWithProvider.CustomerHeaders, before.Stream)

			// 从 Key 池获取可用 Key
			var keyID uint
			if keyPool != nil {
				keyFromPool, kid, err := keyPool.Pick(ctx, provider.ID)
				if err != nil {
					slog.Warn("key pool pick failed", "provider", provider.Name, "error", err)
				} else {
					keyID = kid
					log.ProviderKeyID = keyID
					switch style {
					case consts.StyleAnthropic:
						header.Set("x-api-key", keyFromPool)
					default:
						header.Set("Authorization", fmt.Sprintf("Bearer %s", keyFromPool))
					}
				}
			}

			reqStart := time.Now()
			trace := &httptrace.ClientTrace{
				GotFirstResponseByte: func() {
					fmt.Printf("鍝嶅簲鏃堕棿: %v", time.Since(reqStart))
				},
			}

			req, err := chatModel.BuildReq(httptrace.WithClientTrace(ctx, trace), header, modelWithProvider.ProviderModel, before.raw)
			if err != nil {
				retryLog <- log.WithError(err)
				// 鏋勫缓璇锋眰澶辫触 绉婚櫎寰呴€?
				balancer.Delete(id)
				if err := cooldownManager.OnError(ctx, modelWithProvider, cooldown.CategoryProvider); err != nil {
					slog.Error("update cooldown error", "error", err)
				}
				if keyID > 0 {
					if err := keyPool.OnError(ctx, keyID, cooldown.CategoryProvider); err != nil {
						slog.Error("key pool on error", "error", err)
					}
				}
				continue
			}

			// 将 stream context 附加到请求上下文，用于流处理时的错误处理
			req = req.WithContext(withStreamContext(req.Context(), &streamContext{
				modelWithProvider: modelWithProvider,
				cooldownManager:   cooldownManager,
				keyPool:           keyPool,
				keyID:             keyID,
			}))

			res, err := client.Do(req)
			if err != nil {
				retryLog <- log.WithError(err)
				// 璇锋眰澶辫触 绉婚櫎寰呴€?
				balancer.Delete(id)
				if err := cooldownManager.OnError(ctx, modelWithProvider, cooldown.CategoryProvider); err != nil {
					slog.Error("update cooldown error", "error", err)
				}
				if keyID > 0 {
					if err := keyPool.OnError(ctx, keyID, cooldown.CategoryProvider); err != nil {
						slog.Error("key pool on error", "error", err)
					}
				}
				continue
			}

			if res.StatusCode != http.StatusOK {
				byteBody, err := io.ReadAll(res.Body)
				if err != nil {
					slog.Error("read body error", "error", err)
				}
				retryLog <- log.WithError(fmt.Errorf("status: %d, body: %s", res.StatusCode, string(byteBody)))

				category := cooldown.ClassifyStatus(res.StatusCode)
				if err := cooldownManager.OnError(ctx, modelWithProvider, category); err != nil {
					slog.Error("update cooldown error", "error", err)
				}
				if keyID > 0 {
					if err := keyPool.OnError(ctx, keyID, category); err != nil {
						slog.Error("key pool on error", "error", err)
					}
				}

				if category == cooldown.CategoryKey {
					// 杈惧埌RPM闄愬埗 闄嶄綆鏉冮噸
					balancer.Reduce(id)
				} else {
					// 闈濺PM闄愬埗 绉婚櫎寰呴€?
					balancer.Delete(id)
				}
				res.Body.Close()
				continue
			}

			logId, err := SaveChatLog(ctx, log)
			if err != nil {
				res.Body.Close()
				return nil, 0, err
			}

			return res, logId, nil
		}
	}

	return nil, 0, errors.New("maximum retry attempts reached")
}

func RecordRetryLog(ctx context.Context, retryLog chan models.ChatLog) {
	for log := range retryLog {
		if _, err := SaveChatLog(ctx, log); err != nil {
			slog.Error("save chat log error", "error", err)
		}
	}
}

func RecordLog(ctx context.Context, reqStart time.Time, reader io.ReadCloser, processer Processer, logId uint, before Before, ioLog bool) {
	streamCtx := streamContextFrom(ctx)
	recordFunc := func() error {
		defer reader.Close()
		// 使用独立 context，避免请求结束后 context 被取消导致数据库更新失败
		bgCtx := context.Background()
		if ioLog {
			if err := gorm.G[models.ChatIO](models.DB).Create(bgCtx, &models.ChatIO{
				Input: string(before.raw),
				LogId: logId,
			}); err != nil {
				return err
			}
		}
		log, output, err := processer(bgCtx, reader, before.Stream, reqStart)
		if err != nil {
			handleStreamError(bgCtx, streamCtx, err)
			// 更新 ChatLog 状态为错误
			if _, updateErr := gorm.G[models.ChatLog](models.DB).Where("id = ?", logId).Updates(bgCtx, models.ChatLog{
				Status: "error",
				Error:  err.Error(),
			}); updateErr != nil {
				slog.Error("update chat log error status failed", "error", updateErr)
			}
			return err
		}

		handleStreamSuccess(bgCtx, streamCtx)

		// 使用 map 更新以确保零值也能被更新
		promptDetailsJSON, _ := json.Marshal(log.PromptTokensDetails)
		updates := map[string]interface{}{
			"first_chunk_time":      log.FirstChunkTime,
			"chunk_time":            log.ChunkTime,
			"tps":                   log.Tps,
			"size":                  log.Size,
			"prompt_tokens":         log.PromptTokens,
			"completion_tokens":     log.CompletionTokens,
			"total_tokens":          log.TotalTokens,
			"prompt_tokens_details": string(promptDetailsJSON),
		}
		if err := models.DB.WithContext(bgCtx).Model(&models.ChatLog{}).Where("id = ?", logId).Updates(updates).Error; err != nil {
			return err
		}
		if ioLog {
			if _, err := gorm.G[models.ChatIO](models.DB).Where("log_id = ?", logId).Updates(bgCtx, models.ChatIO{OutputUnion: *output}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := recordFunc(); err != nil {
		slog.Error("record log error", "error", err)
	}
}

func handleStreamSuccess(ctx context.Context, streamCtx *streamContext) {
	if streamCtx == nil {
		return
	}
	if err := streamCtx.cooldownManager.OnSuccess(ctx, streamCtx.modelWithProvider); err != nil {
		slog.Error("clear cooldown error", "error", err)
	}
	if streamCtx.keyID > 0 && streamCtx.keyPool != nil {
		if err := streamCtx.keyPool.OnSuccess(ctx, streamCtx.keyID); err != nil {
			slog.Error("key pool on success", "error", err)
		}
	}
}

func handleStreamError(ctx context.Context, streamCtx *streamContext, processErr error) {
	if streamCtx == nil {
		return
	}
	category := classifyStreamError(processErr)
	if err := streamCtx.cooldownManager.OnError(ctx, streamCtx.modelWithProvider, category); err != nil {
		slog.Error("update cooldown error", "error", err)
	}
	if streamCtx.keyID > 0 && streamCtx.keyPool != nil {
		if err := streamCtx.keyPool.OnError(ctx, streamCtx.keyID, category); err != nil {
			slog.Error("key pool on error", "error", err)
		}
	}
}

func classifyStreamError(err error) cooldown.Category {
	var streamErr StreamError
	switch {
	case errors.As(err, &streamErr):
		if streamErr.Category != cooldown.CategoryNone {
			return streamErr.Category
		}
		return cooldown.CategoryProvider
	case errors.Is(err, context.Canceled):
		return cooldown.CategoryClient
	case errors.Is(err, context.DeadlineExceeded):
		return cooldown.CategoryProvider
	default:
		return cooldown.CategoryProvider
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
	ModelWithProviderMap map[uint]*models.ModelWithProvider
	WeightItems          map[uint]int
	ProviderMap          map[uint]models.Provider
	MaxRetry             int
	TimeOut              int
	IOLog                bool
	Strategy             string // 璐熻浇鍧囪　绛栫暐
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

	modelWithProviderMap := make(map[uint]*models.ModelWithProvider, len(modelWithProviders))
	for i := range modelWithProviders {
		mp := &modelWithProviders[i]
		modelWithProviderMap[mp.ID] = mp
	}

	providers, err := gorm.G[models.Provider](models.DB).
		Where("id IN ?", lo.Map(modelWithProviders, func(mp models.ModelWithProvider, _ int) uint { return mp.ProviderID })).
		Where("type = ?", style).
		Find(ctx)
	if err != nil {
		return nil, err
	}

	providerMap := lo.KeyBy(providers, func(p models.Provider) uint { return p.ID })

	weightItems := make(map[uint]int)
	for _, mp := range modelWithProviders {
		if _, ok := providerMap[mp.ProviderID]; !ok {
			continue
		}
		weightItems[mp.ID] = mp.Weight
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
