package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"
	"time"

	"github.com/atopos31/llmio/models"
	"github.com/atopos31/llmio/service/cooldown"
	"github.com/tidwall/gjson"
)

const (
	InitScannerBufferSize = 1024 * 8         // 8KB
	MaxScannerBufferSize  = 1024 * 1024 * 64 // 64MB
)

type Processer func(ctx context.Context, pr io.Reader, stream bool, start time.Time) (*models.ChatLog, *models.OutputUnion, error)

// StreamError SSE 流中的结构化错误
type StreamError struct {
	Message  string
	Type     string
	Code     string
	Status   int
	Category cooldown.Category
}

func (e StreamError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "stream error"
	}
	if e.Code != "" {
		msg += " code=" + e.Code
	}
	if e.Type != "" {
		msg += " type=" + e.Type
	}
	if e.Status != 0 {
		msg += fmt.Sprintf(" status=%d", e.Status)
	}
	return msg
}

func (e *StreamError) resolveCategory() {
	if e.Status != 0 {
		e.Category = cooldown.ClassifyStatus(e.Status)
		return
	}
	// 根据错误代码和类型分类
	switch {
	case e.Code == "insufficient_quota" || e.Code == "invalid_api_key" ||
		e.Code == "rate_limit_exceeded" || e.Code == "billing_hard_limit_reached" ||
		e.Code == "quota_exceeded" || e.Code == "authentication_error":
		e.Category = cooldown.CategoryKey
	case strings.HasPrefix(e.Type, "server_error") || e.Type == "overloaded_error":
		e.Category = cooldown.CategoryProvider
	case e.Type == "invalid_request_error":
		e.Category = cooldown.CategoryClient
	default:
		e.Category = cooldown.CategoryProvider
	}
}

func parseStreamError(chunk string) error {
	errStr := gjson.Get(chunk, "error")
	if !errStr.Exists() {
		return nil
	}
	streamErr := StreamError{
		Message: errStr.Get("message").String(),
		Type:    errStr.Get("type").String(),
		Code:    errStr.Get("code").String(),
		Status:  int(errStr.Get("status").Int()),
	}
	if streamErr.Message == "" {
		streamErr.Message = errStr.String()
	}
	streamErr.resolveCategory()
	return streamErr
}

func ProcesserOpenAI(ctx context.Context, pr io.Reader, stream bool, start time.Time) (*models.ChatLog, *models.OutputUnion, error) {
	// 首字时延
	var firstChunkTime time.Duration
	var once sync.Once

	var usageStr string
	var output models.OutputUnion
	var size int

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, InitScannerBufferSize), MaxScannerBufferSize)
	for chunk, chunkSize := range ScannerToken(scanner) {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		size += chunkSize
		once.Do(func() {
			firstChunkTime = time.Since(start)
		})
		if !stream {
			output.OfString = chunk
			usageStr = gjson.Get(chunk, "usage").String()
			break
		}
		chunk = strings.TrimPrefix(chunk, "data: ")
		if chunk == "[DONE]" {
			break
		}
		if err := parseStreamError(chunk); err != nil {
			return nil, nil, err
		}
		output.OfStringArray = append(output.OfStringArray, chunk)

		// 部分厂商openai格式中 每段sse响应都会返回usage 兼容性考虑
		// if usageStr != "" {
		// 	break
		// }

		usage := gjson.Get(chunk, "usage")
		if usage.Exists() && usage.Get("total_tokens").Int() != 0 {
			usageStr = usage.String()
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	// token用量
	var openaiUsage models.Usage
	usage := []byte(usageStr)
	if json.Valid(usage) {
		if err := json.Unmarshal(usage, &openaiUsage); err != nil {
			return nil, nil, err
		}
	}

	chunkTime := time.Since(start) - firstChunkTime

	return &models.ChatLog{
		FirstChunkTime: firstChunkTime,
		ChunkTime:      chunkTime,
		Usage:          openaiUsage,
		Tps:            float64(openaiUsage.TotalTokens) / chunkTime.Seconds(),
		Size:           size,
	}, &output, nil
}

type OpenAIResUsage struct {
	InputTokens        int64              `json:"input_tokens"`
	OutputTokens       int64              `json:"output_tokens"`
	TotalTokens        int64              `json:"total_tokens"`
	InputTokensDetails InputTokensDetails `json:"input_tokens_details"`
}

type InputTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

type AnthropicUsage struct {
	InputTokens              int64  `json:"input_tokens"`
	CacheCreationInputTokens int64  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64  `json:"cache_read_input_tokens"`
	OutputTokens             int64  `json:"output_tokens"`
	ServiceTier              string `json:"service_tier"`
}

func mergeAnthropicUsage(target *AnthropicUsage, source gjson.Result) {
	if v := source.Get("input_tokens").Int(); v > 0 {
		target.InputTokens += v
	}
	if v := source.Get("cache_creation_input_tokens").Int(); v > 0 {
		target.CacheCreationInputTokens += v
	}
	if v := source.Get("cache_read_input_tokens").Int(); v > 0 {
		target.CacheReadInputTokens += v
	}
	if v := source.Get("output_tokens").Int(); v > 0 {
		target.OutputTokens += v
	}
}

func ProcesserOpenAiRes(ctx context.Context, pr io.Reader, stream bool, start time.Time) (*models.ChatLog, *models.OutputUnion, error) {
	// 首字时延
	var firstChunkTime time.Duration
	var once sync.Once

	var usageStr string
	var output models.OutputUnion
	var size int

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, InitScannerBufferSize), MaxScannerBufferSize)
	var event string
	for chunk, chunkSize := range ScannerToken(scanner) {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		size += chunkSize
		once.Do(func() {
			firstChunkTime = time.Since(start)
		})
		if !stream {
			output.OfString = chunk
			usageStr = gjson.Get(chunk, "usage").String()
			break
		}

		if after, ok := strings.CutPrefix(chunk, "event: "); ok {
			event = after
			continue
		}
		content := strings.TrimPrefix(chunk, "data: ")
		if content == "" {
			continue
		}
		if err := parseStreamError(content); err != nil {
			return nil, nil, err
		}
		output.OfStringArray = append(output.OfStringArray, content)
		if event == "response.completed" {
			usageStr = gjson.Get(content, "response.usage").String()
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	var openAIResUsage OpenAIResUsage
	usage := []byte(usageStr)
	if json.Valid(usage) {
		if err := json.Unmarshal(usage, &openAIResUsage); err != nil {
			return nil, nil, err
		}
	}

	chunkTime := time.Since(start) - firstChunkTime

	return &models.ChatLog{
		FirstChunkTime: firstChunkTime,
		ChunkTime:      chunkTime,
		Usage: models.Usage{
			PromptTokens:     openAIResUsage.InputTokens,
			CompletionTokens: openAIResUsage.OutputTokens,
			TotalTokens:      openAIResUsage.TotalTokens,
			PromptTokensDetails: models.PromptTokensDetails{
				CachedTokens: openAIResUsage.InputTokensDetails.CachedTokens,
			},
		},
		Tps:  float64(openAIResUsage.TotalTokens) / chunkTime.Seconds(),
		Size: size,
	}, &output, nil
}

func ProcesserAnthropic(ctx context.Context, pr io.Reader, stream bool, start time.Time) (*models.ChatLog, *models.OutputUnion, error) {
	// 首字时延
	var firstChunkTime time.Duration
	var once sync.Once

	var athropicUsage AnthropicUsage

	var output models.OutputUnion
	var size int

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, InitScannerBufferSize), MaxScannerBufferSize)
	for chunk, chunkSize := range ScannerToken(scanner) {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}
		size += chunkSize
		once.Do(func() {
			firstChunkTime = time.Since(start)
		})
		if !stream {
			output.OfString = chunk
			if usageStr := gjson.Get(chunk, "usage").String(); usageStr != "" {
				usage := []byte(usageStr)
				if json.Valid(usage) {
					json.Unmarshal(usage, &athropicUsage)
				}
			}
			break
		}

		if _, ok := strings.CutPrefix(chunk, "event: "); ok {
			continue
		}

		after, ok := strings.CutPrefix(chunk, "data: ")
		if !ok {
			continue
		}

		if err := parseStreamError(after); err != nil {
			return nil, nil, err
		}
		output.OfStringArray = append(output.OfStringArray, after)

		// 从所有事件中提取 usage 并合并
		if topUsage := gjson.Get(after, "usage"); topUsage.Exists() {
			mergeAnthropicUsage(&athropicUsage, topUsage)
		}
		if msgUsage := gjson.Get(after, "message.usage"); msgUsage.Exists() {
			mergeAnthropicUsage(&athropicUsage, msgUsage)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	chunkTime := time.Since(start) - firstChunkTime
	totalTokens := athropicUsage.InputTokens + athropicUsage.OutputTokens

	var tps float64
	if chunkTime.Seconds() > 0 {
		tps = float64(totalTokens) / chunkTime.Seconds()
	}

	return &models.ChatLog{
		FirstChunkTime: firstChunkTime,
		ChunkTime:      chunkTime,
		Usage: models.Usage{
			PromptTokens:     athropicUsage.InputTokens,
			CompletionTokens: athropicUsage.OutputTokens,
			TotalTokens:      totalTokens,
			PromptTokensDetails: models.PromptTokensDetails{
				CachedTokens: athropicUsage.CacheReadInputTokens,
			},
		},
		Tps:  tps,
		Size: size,
	}, &output, nil
}

func ScannerToken(reader *bufio.Scanner) iter.Seq2[string, int] {
	return func(yield func(string, int) bool) {
		for reader.Scan() {
			chunk := reader.Text()
			if chunk == "" {
				continue
			}
			if !yield(chunk, len(reader.Bytes())) {
				return
			}
		}
	}
}
