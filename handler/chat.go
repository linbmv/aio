package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
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

func ChatCompletionsHandler(c *gin.Context) { chatHandler(c, consts.StyleOpenAI) }
func ResponsesHandler(c *gin.Context)       { chatHandler(c, consts.StyleOpenAIRes) }
func Messages(c *gin.Context)               { chatHandler(c, consts.StyleAnthropic) }

func chatHandler(c *gin.Context, defaultFormat string) {
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.InternalServerError(c, err.Error())
		return
	}
	c.Request.Body.Close()

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

	writeHeader(c, before.Stream, res.Header)

	if before.Stream {
		pr, pw := io.Pipe()
		reader := io.TeeReader(res.Body, pw)
		go func() {
			defer res.Body.Close()
			defer pw.Close()
			if err := formatx.ConvertStream(reader, c.Writer, providerType, requestFormat, before.Model); err != nil {
				pw.CloseWithError(err)
			}
		}()
		go service.RecordLog(context.Background(), startReq, pr, logProcessor, logId, *before, providersWithMeta.IOLog)
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
