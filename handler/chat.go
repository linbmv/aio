package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/atopos31/llmio/common"
	"github.com/atopos31/llmio/consts"
	"github.com/atopos31/llmio/models"
	"github.com/atopos31/llmio/service"
	"github.com/atopos31/llmio/service/cache"
	"github.com/gin-gonic/gin"
)

var (
	// chatCache 全局缓存实例，按AuthKeyID和模型隔离
	chatCache cache.Cache = cache.NewMemoryCache(1024)
	// chatCacheTTL 默认缓存有效期
	chatCacheTTL = time.Minute * 5
)

func ChatCompletionsHandler(c *gin.Context) {
	chatHandler(c, service.BeforerOpenAI, service.ProcesserOpenAI, consts.StyleOpenAI)
}

func ResponsesHandler(c *gin.Context) {
	chatHandler(c, service.BeforerOpenAIRes, service.ProcesserOpenAiRes, consts.StyleOpenAIRes)
}

func Messages(c *gin.Context) {
	chatHandler(c, service.BeforerAnthropic, service.ProcesserAnthropic, consts.StyleAnthropic)
}

func chatHandler(c *gin.Context, preProcessor service.Beforer, postProcessor service.Processer, style string) {
	// 读取原始请求体
	reqBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}
	c.Request.Body.Close()
	// 预处理、提取模型参数
	before, err := preProcessor(reqBody)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}

	ctx := c.Request.Context()
	// 校验 authKey 是否有权限使用该模型
	valid, err := validateAuthKey(ctx, before.Model)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}
	if !valid {
		common.ErrorWithHttpStatus(c, http.StatusForbidden, http.StatusForbidden, "auth key has no permission to use this model")
		return
	}

	// 尝试从缓存获取响应（仅对非流式请求）
	cacheKey, cacheEnabled := service.BuildCacheKey(ctx, style, *before)
	if cacheEnabled && chatCache != nil {
		if cached, hit, err := chatCache.Get(ctx, cacheKey); err == nil && hit {
			// 缓存命中，记录审计日志
			reqMeta := models.ReqMeta{
				Header:    c.Request.Header,
				RemoteIP:  c.ClientIP(),
				UserAgent: c.Request.UserAgent(),
			}
			service.RecordCacheHit(ctx, cacheKey, cached, reqMeta)

			// 直接返回已缓存的响应
			writeCachedResponse(c, cached)
			return
		}
	}
	// 按模型获取可用 provider
	providersWithMeta, err := service.ProvidersWithMetaBymodelsName(ctx, style, *before)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}

	startReq := time.Now()
	// 调用负载均衡后的 provider 并转发
	res, logId, err := service.BalanceChat(ctx, startReq, style, *before, *providersWithMeta, models.ReqMeta{
		Header:    c.Request.Header,
		RemoteIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}
	defer res.Body.Close()

	// 处理响应流，同时支持缓存写入
	pr, pw := io.Pipe()
	var reader io.Reader = res.Body
	var buf bytes.Buffer

	if !before.Stream && cacheEnabled && chatCache != nil {
		// 非流式请求：同时写入管道和缓存缓冲区
		reader = io.TeeReader(reader, pw)
		reader = io.TeeReader(reader, &buf)
	} else {
		// 流式请求或缓存未启用：仅写入管道
		reader = io.TeeReader(reader, pw)
	}

	// 异步处理输出并记录 tokens
	go service.RecordLog(service.CopyStreamContext(res.Request.Context()), startReq, pr, postProcessor, logId, *before, providersWithMeta.IOLog)

	writeHeader(c, before.Stream, res.Header)
	c.Status(res.StatusCode)
	if _, err := io.Copy(c.Writer, reader); err != nil {
		pw.CloseWithError(err)
		common.InternalServerError(c, err.Error())
		return
	}

	pw.Close()

	// 非流式请求完成后写入缓存
	if !before.Stream && cacheEnabled && chatCache != nil && buf.Len() > 0 {
		cacheValue := &cache.Value{
			StatusCode:    res.StatusCode,
			Header:        res.Header.Clone(),
			Body:          buf.Bytes(),
			CreatedAt:     time.Now(),
			SourceLogID:   logId,
			ProviderName:  getProviderName(providersWithMeta),
			ProviderModel: before.Model,
		}
		// 异步写入缓存，避免阻塞响应
		go func() {
			_ = chatCache.Set(context.Background(), cacheKey, cacheValue, chatCacheTTL)
		}()
	}
}

func writeHeader(c *gin.Context, stream bool, header http.Header) {
	for k, values := range header {
		for _, value := range values {
			c.Writer.Header().Add(k, value)
		}
	}

	if stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
	}
	c.Writer.Flush()
}

// 校验auhtKey的模型使用权限
func validateAuthKey(ctx context.Context, model string) (bool, error) {
	// 验证是否为允许全部模型
	allowAll, ok := ctx.Value(consts.ContextKeyAllowAllModel).(bool)
	if !ok {
		return false, errors.New("invalid auth key")
	}
	if allowAll {
		return true, nil
	}
	// 验证是否有权限使用该模型
	allowedModels, ok := ctx.Value(consts.ContextKeyAllowModels).([]string)
	if !ok {
		return false, errors.New("invalid auth key")
	}
	return slices.Contains(allowedModels, model), nil
}

// writeCachedResponse 写入缓存的响应数据
func writeCachedResponse(c *gin.Context, cached *cache.Value) {
	// 复制必要的响应头
	for k, values := range cached.Header {
		for _, value := range values {
			c.Writer.Header().Add(k, value)
		}
	}

	// 添加缓存标识头
	c.Header("X-Cache", "HIT")
	c.Header("X-Cache-Created", cached.CreatedAt.Format(time.RFC3339))

	c.Status(cached.StatusCode)
	if _, err := c.Writer.Write(cached.Body); err != nil {
		common.InternalServerError(c, err.Error())
	}
}

// getProviderName 从 ProvidersWithMeta 中获取 Provider 名称
func getProviderName(providersWithMeta *service.ProvidersWithMeta) string {
	if providersWithMeta == nil || len(providersWithMeta.ProviderMap) == 0 {
		return "unknown"
	}

	// 取第一个 Provider 的名称作为代表
	for _, provider := range providersWithMeta.ProviderMap {
		return provider.Name
	}
	return "unknown"
}
