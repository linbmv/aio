package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/atopos31/llmio/common"
	"github.com/atopos31/llmio/consts"
	"github.com/atopos31/llmio/models"
	"github.com/atopos31/llmio/service"
	"github.com/atopos31/llmio/service/formatx"
	"github.com/gin-gonic/gin"
)

var debugMode = os.Getenv("DEBUG_MODE") == "true"

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
	slog.Info("chat handler start", "format", requestFormat, "model", before.Model)

	providersWithMeta, err := service.ProvidersWithMetaBymodelsName(ctx, requestFormat, *before)
	if err != nil {
		slog.Error("failed to get providers", "error", err)
		common.InternalServerError(c, err.Error())
		return
	}
	slog.Info("providers loaded", "count", len(providersWithMeta.ProviderMap))

	startReq := time.Now()
	res, logId, providerType, err := service.BalanceChat(ctx, startReq, requestFormat, *before, *providersWithMeta, models.ReqMeta{
		Header:    c.Request.Header,
		RemoteIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		slog.Error("balance chat failed", "error", err)
		common.InternalServerError(c, err.Error())
		return
	}
	slog.Info("got response from provider", "status", res.StatusCode, "provider", providerType)

	logProcessor := postProcessors[providerType]
	if logProcessor == nil {
		logProcessor = postProcessors[requestFormat]
	}

	c.Status(res.StatusCode)
	writeHeader(c, before.Stream, res.Header)

	if before.Stream {
		slog.Info("starting stream response")
		if debugMode {
			slog.Debug("response headers", "headers", sanitizeHeaders(res.Header))
			slog.Debug("client headers", "headers", sanitizeHeaders(c.Request.Header))
		}

		// 使用独立的context避免请求context取消影响流式响应
		streamCtx := context.Background()

		pr, pw := io.Pipe()
		defer pw.Close()
		reader := io.TeeReader(res.Body, pw)
		defer res.Body.Close()

		go service.RecordLog(context.Background(), startReq, pr, logProcessor, logId, *before, providersWithMeta.IOLog)

		slog.Info("starting stream conversion")
		if debugMode {
			slog.Debug("convert stream", "providerType", providerType, "requestFormat", requestFormat, "model", before.Model)
		}

		if err := formatx.ConvertStream(streamCtx, reader, c.Writer, providerType, requestFormat, before.Model, debugMode); err != nil {
			pw.CloseWithError(err)
			if !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrAbortHandler) && !errors.Is(err, io.ErrClosedPipe) {
				slog.Error("convert stream error", "error", err)
			}
			return
		}
		slog.Info("stream conversion completed successfully")
		pw.Close()
		return
	}

	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}
	if debugMode {
		slog.Debug("non-stream response read", "bytes", len(bodyBytes))
	}
	go service.RecordLog(context.Background(), startReq, io.NopCloser(bytes.NewReader(bodyBytes)), logProcessor, logId, *before, providersWithMeta.IOLog)

	respBody, err := formatx.ConvertResponse(bodyBytes, providerType, requestFormat, before.Model)
	if err != nil {
		slog.Error("convert response failed", "error", err)
		common.InternalServerError(c, err.Error())
		return
	}
	if debugMode {
		slog.Debug("response converted", "original_bytes", len(bodyBytes), "converted_bytes", len(respBody))
	}
	c.Writer.Header().Del("Content-Length")
	c.Header("Content-Length", fmt.Sprintf("%d", len(respBody)))
	written, err := c.Writer.Write(respBody)
	if err != nil {
		slog.Error("write response failed", "bytes", written, "error", err)
		common.InternalServerError(c, err.Error())
		return
	}
	if debugMode {
		slog.Debug("response written to client", "bytes", written)
	}
}

func writeHeader(c *gin.Context, stream bool, header http.Header) {
	// hop-by-hop headers that should not be forwarded
	hopByHop := map[string]bool{
		"connection":        true,
		"proxy-connection": true,
		"keep-alive":        true,
		"transfer-encoding": true,
		"upgrade":           true,
		"te":                true,
		"trailer":           true,
	}

	// Copy headers from upstream, excluding hop-by-hop and problematic headers
	for k, values := range header {
		lowerKey := strings.ToLower(k)
		if hopByHop[lowerKey] {
			continue
		}
		// For streaming, remove Content-Length and Content-Encoding
		if stream && (lowerKey == "content-length" || lowerKey == "content-encoding") {
			continue
		}
		// Use Set instead of Add to avoid duplicate headers
		c.Writer.Header()[k] = append([]string(nil), values...)
	}

	if stream {
		// Disable gzip for SSE streams
		c.Set("gzip-abort", true)
		// Set SSE headers
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		// Ensure no Content-Length
		c.Writer.Header().Del("Content-Length")
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

// sanitizeHeaders 屏蔽敏感头部信息用于日志记录
func sanitizeHeaders(headers http.Header) map[string]string {
	sensitiveKeys := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"set-cookie":    true,
		"api-key":       true,
		"x-api-key":     true,
	}

	safe := make(map[string]string)
	for k, v := range headers {
		lowerKey := strings.ToLower(k)
		if sensitiveKeys[lowerKey] {
			safe[k] = "[REDACTED]"
		} else if len(v) > 0 {
			safe[k] = v[0]
		}
	}
	return safe
}
