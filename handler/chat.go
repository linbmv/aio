package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/atopos31/llmio/common"
	"github.com/atopos31/llmio/consts"
	"github.com/atopos31/llmio/models"
	"github.com/atopos31/llmio/service"
	"github.com/atopos31/llmio/service/formatx"
	"github.com/gin-gonic/gin"
)

var (
	preProcessors = map[string]service.Beforer{
		consts.StyleOpenAI:    service.BeforerOpenAI,
		consts.StyleOpenAIRes: service.BeforerOpenAIRes,
		consts.StyleAnthropic: service.BeforerAnthropic,
	}
	postProcessors = map[string]service.Processer{
		consts.StyleOpenAI:    service.ProcesserOpenAI,
		consts.StyleOpenAIRes: service.ProcesserOpenAiRes,
		consts.StyleAnthropic: service.ProcesserAnthropic,
	}
)

// UnifiedChat 统一入口，根据路由兜底格式后让请求体检测覆盖
func UnifiedChat(c *gin.Context) { chatHandler(c, defaultFormatFromPath(c.FullPath())) }

func ChatCompletionsHandler(c *gin.Context) { UnifiedChat(c) }
func ResponsesHandler(c *gin.Context)       { UnifiedChat(c) }
func Messages(c *gin.Context)               { UnifiedChat(c) }

func chatHandler(c *gin.Context, defaultFormat string) {
	defer c.Request.Body.Close()

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			common.PayloadTooLarge(c, "request body too large")
			return
		}
		common.InternalServerError(c, err.Error())
		return
	}

	requestFormat := formatx.DetectFormat(rawBody, defaultFormat)
	preProcessor := preProcessors[requestFormat]
	if preProcessor == nil {
		common.InternalServerError(c, "unsupported request format")
		return
	}
	before, err := preProcessor(rawBody)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}

	ctx := c.Request.Context()
	providersWithMeta, err := service.ProvidersWithMetaBymodelsName(ctx, requestFormat, *before)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}

	startReq := time.Now()
	res, logId, providerType, err := service.BalanceChat(ctx, startReq, requestFormat, *before, *providersWithMeta, models.ReqMeta{
		Header:    c.Request.Header,
		RemoteIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}

	logProcessor := postProcessors[providerType]
	if logProcessor == nil {
		logProcessor = postProcessors[requestFormat]
	}

	c.Status(res.StatusCode)
	writeHeader(c, before.Stream, res.Header)

	if before.Stream {
		// 流式响应保持 handler 存活，避免请求 context 被提前取消导致转换中断
		streamCtx, streamCancel := context.WithCancel(context.WithoutCancel(ctx))
		defer streamCancel()

		go func() {
			<-ctx.Done()
			streamCancel()
		}()

		pr, pw := io.Pipe()
		reader := io.TeeReader(res.Body, pw)
		defer res.Body.Close()

		go service.RecordLog(context.Background(), startReq, pr, logProcessor, logId, *before, providersWithMeta.IOLog)

		if err := formatx.ConvertStream(streamCtx, reader, c.Writer, providerType, requestFormat, before.Model); err != nil {
			pw.CloseWithError(err)
			if !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrAbortHandler) && !errors.Is(err, io.ErrClosedPipe) {
				slog.Error("convert stream error", "error", err)
			}
			return
		}
		pw.Close()
		return
	}

	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}
	go service.RecordLog(context.Background(), startReq, io.NopCloser(bytes.NewReader(bodyBytes)), logProcessor, logId, *before, providersWithMeta.IOLog)

	respBody, err := formatx.ConvertResponse(bodyBytes, providerType, requestFormat, before.Model)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}
	c.Writer.Header().Del("Content-Length")
	c.Header("Content-Length", fmt.Sprintf("%d", len(respBody)))
	if _, err := c.Writer.Write(respBody); err != nil {
		common.InternalServerError(c, err.Error())
		return
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

func defaultFormatFromPath(path string) string {
	switch {
	case strings.Contains(path, "/responses"):
		return consts.StyleOpenAIRes
	case strings.Contains(path, "/messages"):
		return consts.StyleAnthropic
	default:
		return consts.StyleOpenAI
	}
}
