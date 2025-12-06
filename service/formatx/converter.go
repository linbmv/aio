package formatx

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/atopos31/llmio/consts"
	"github.com/tidwall/gjson"
)

// normalizeProviderStyle 将 openai-res Provider 视为 openai，避免把客户端格式误当 Provider 类型
func normalizeProviderStyle(style string) string {
	if style == consts.StyleOpenAIRes {
		return consts.StyleOpenAI
	}
	return style
}

// safeFlush 在刷新过程中捕获 panic，避免 SSE 连接异常导致进程崩溃
func safeFlush(w io.Writer) {
	if f, ok := w.(http.Flusher); ok {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("safeFlush panic recovered", "panic", r)
			}
		}()
		f.Flush()
	}
}

// OpenAIToAnthropic 将OpenAI 请求转换为Anthropic 格式（含多模态与tools）
func OpenAIToAnthropic(raw []byte) ([]byte, error) {
	model := gjson.GetBytes(raw, "model").String()
	stream := gjson.GetBytes(raw, "stream").Bool()
	maxTokens := gjson.GetBytes(raw, "max_tokens").Int()
	temp := gjson.GetBytes(raw, "temperature").Float()

	var system string
	var messages []map[string]any
	gjson.GetBytes(raw, "messages").ForEach(func(_, m gjson.Result) bool {
		role := m.Get("role").String()
		content := m.Get("content")
		if role == "system" {
			system = content.String()
			return true
		}
		var blocks []map[string]any
		if content.IsArray() {
			content.ForEach(func(_, c gjson.Result) bool {
				switch c.Get("type").String() {
				case "text":
					blocks = append(blocks, map[string]any{"type": "text", "text": c.Get("text").String()})
				case "image_url":
					blocks = append(blocks, map[string]any{
						"type":   "image",
						"source": map[string]any{"type": "base64", "media_type": c.Get("image_url.detail").String(), "data": c.Get("image_url.url").String()},
					})
				}
				return true
			})
		} else {
			blocks = append(blocks, map[string]any{"type": "text", "text": content.String()})
		}
		messages = append(messages, map[string]any{"role": role, "content": blocks})
		return true
	})

	var tools []map[string]any
	gjson.GetBytes(raw, "tools").ForEach(func(_, t gjson.Result) bool {
		if t.Get("type").String() != "function" {
			return true
		}
		tools = append(tools, map[string]any{
			"name":         t.Get("function.name").String(),
			"description":  t.Get("function.description").String(),
			"input_schema": t.Get("function.parameters").Value(),
		})
		return true
	})

	payload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	}
	if system != "" {
		payload["system"] = system
	}
	if maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}
	if temp > 0 {
		payload["temperature"] = temp
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	return json.Marshal(payload)
}

// AnthropicToOpenAIReq 将Anthropic 请求转换为OpenAI 格式（保留多模态）
func AnthropicToOpenAIReq(raw []byte) ([]byte, error) {
	system := gjson.GetBytes(raw, "system").String()
	model := gjson.GetBytes(raw, "model").String()
	stream := gjson.GetBytes(raw, "stream").Bool()

	var messages []map[string]any
	if system != "" {
		messages = append(messages, map[string]any{"role": "system", "content": system})
	}

	gjson.GetBytes(raw, "messages").ForEach(func(_, msg gjson.Result) bool {
		role := msg.Get("role").String()
		content := msg.Get("content")
		if !content.IsArray() {
			messages = append(messages, map[string]any{"role": role, "content": content.String()})
			return true
		}
		var blocks []map[string]any
		content.ForEach(func(_, c gjson.Result) bool {
			switch c.Get("type").String() {
			case "text":
				blocks = append(blocks, map[string]any{"type": "text", "text": c.Get("text").String()})
			case "image":
				blocks = append(blocks, map[string]any{"type": "image_url", "image_url": map[string]any{"url": c.Get("source.data").String(), "detail": c.Get("source.media_type").String()}})
			}
			return true
		})
		messages = append(messages, map[string]any{"role": role, "content": blocks})
		return true
	})

	return json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	})
}

// AnthropicToOpenAI 将Anthropic 响应转换为OpenAI 格式（含tool_calls/finish_reason/usage）
func AnthropicToOpenAI(raw []byte, model string) ([]byte, error) {
	id := gjson.GetBytes(raw, "id").String()
	stop := gjson.GetBytes(raw, "stop_reason").String()
	content := gjson.GetBytes(raw, "content")

	var textParts []string
	var toolCalls []map[string]any
	content.ForEach(func(_, c gjson.Result) bool {
		switch c.Get("type").String() {
		case "text":
			textParts = append(textParts, c.Get("text").String())
		case "tool_use":
			toolCalls = append(toolCalls, map[string]any{
				"id":   c.Get("id").String(),
				"type": "function",
				"function": map[string]any{
					"name": c.Get("name").String(),
					"arguments": func() string {
						if c.Get("input").Exists() && !c.Get("input").IsArray() {
							b, _ := json.Marshal(c.Get("input").Value())
							return string(b)
						}
						return "{}"
					}(),
				},
			})
		}
		return true
	})

	finish := "stop"
	if stop == "tool_use" {
		finish = "tool_calls"
	} else if stop != "" {
		finish = stop
	}

	usage := map[string]int64{
		"prompt_tokens":     gjson.GetBytes(raw, "usage.input_tokens").Int(),
		"completion_tokens": gjson.GetBytes(raw, "usage.output_tokens").Int(),
	}
	usage["total_tokens"] = usage["prompt_tokens"] + usage["completion_tokens"]

	msg := map[string]any{
		"role":    "assistant",
		"content": strings.Join(textParts, ""),
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}

	resp := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{"index": 0, "message": msg, "finish_reason": finish}},
		"usage":   usage,
	}
	return json.Marshal(resp)
}

// AnthropicSSEToOpenAI 将 Anthropic SSE 流转换为 OpenAI SSE 格式
func AnthropicSSEToOpenAI(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string
	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
	created := time.Now().Unix()
	firstChunk := true
	var usage map[string]any

	flush := func(payload map[string]any) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", b)
		safeFlush(w)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch eventType {
		case "message_delta":
			if u := gjson.Get(data, "usage"); u.Exists() {
				if usageMap, ok := u.Value().(map[string]any); ok {
					usage = usageMap
				}
			}
		case "content_block_delta":
			text := gjson.Get(data, "delta.text").String()
			if text == "" {
				continue
			}
			if firstChunk {
				flush(map[string]any{
					"id":      chunkID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": []map[string]any{{"index": 0, "delta": map[string]string{"role": "assistant"}, "finish_reason": nil}},
				})
				firstChunk = false
			}
			flush(map[string]any{
				"id":      chunkID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": nil}},
			})
		case "message_stop":
			final := map[string]any{
				"id":      chunkID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
			}
			if usage != nil {
				final["usage"] = usage
			}
			flush(final)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			safeFlush(w)
			return nil
		case "error":
			return fmt.Errorf("anthropic stream error: %s", data)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("anthropic stream read error: %w", err)
	}
	return fmt.Errorf("anthropic stream closed without stop event")
}

// AnthropicSSEToOpenAIRes 将 Anthropic SSE 流转换为 OpenAI-Res SSE 格式
func AnthropicSSEToOpenAIRes(r io.Reader, w io.Writer, model string, debug bool) error {
	// 立即发送ping以唤醒客户端SSE读取器，防止超时
	fmt.Fprintf(w, ": ping\n\n")
	safeFlush(w)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string
	var chunkCount, totalBytes int

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			switch eventType {
			case "content_block_delta":
				text := gjson.Get(data, "delta.text").String()
				if text == "" {
					text = gjson.Get(data, "delta.reasoning_content").String()
				}
				if text != "" {
					chunk := map[string]interface{}{
						"model":  model,
						"output": text,
					}
					chunkBytes, _ := json.Marshal(chunk)
					n, _ := fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
					totalBytes += n
					chunkCount++
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
				}

			case "message_stop":
				fmt.Fprintf(w, "data: [DONE]\n\n")
				safeFlush(w)
				if debug {
					slog.Debug("AnthropicSSEToOpenAIRes completed", "chunks", chunkCount, "bytes", totalBytes)
				}
				return nil

			case "error":
				errMsg := gjson.Get(data, "error.message").String()
				if errMsg == "" {
					errMsg = "unknown error"
				}
				return fmt.Errorf("anthropic stream error: %s", errMsg)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("anthropic stream read error: %w", err)
	}
	// 如果已经发送了数据，优雅结束；否则报错
	if chunkCount > 0 {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		safeFlush(w)
		if debug {
			slog.Debug("AnthropicSSEToOpenAIRes completed without stop event", "chunks", chunkCount)
		}
		return nil
	}
	return fmt.Errorf("anthropic stream closed without stop event")
}

// OpenAIResponsesAPISSEToOpenAIRes 将 OpenAI Responses API 的 SSE 事件流转换为简化的 OpenAI-Res SSE 格式
func OpenAIResponsesAPISSEToOpenAIRes(r io.Reader, w io.Writer, model string, debug bool) error {
	// 立即发送ping以唤醒客户端
	fmt.Fprintf(w, ": ping\n\n")
	safeFlush(w)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string
	var chunkCount, totalBytes int

	for scanner.Scan() {
		line := scanner.Text()

		// 调试：记录所有接收到的行(包括空行)
		if debug {
			if line == "" {
				slog.Info("SSE: received empty line")
			} else {
				slog.Info("SSE: received line", "line", line, "length", len(line))
			}
		}

		// 跳过注释行但不跳过空行(空行是SSE格式的一部分)
		if strings.HasPrefix(line, ":") {
			continue
		}

		// 空行表示事件结束,重置eventType
		if line == "" {
			eventType = ""
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			if debug {
				slog.Info("SSE: event type", "eventType", eventType)
			}
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			if debug {
				slog.Info("SSE: skipping non-data line", "line", line)
			}
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			if debug {
				slog.Info("SSE: empty data field")
			}
			continue
		}
		if debug {
			slog.Info("SSE: processing data", "data", data, "eventType", eventType)
		}

		// 处理带 event 的格式
		if eventType != "" {
			switch eventType {
			case "response.output_text.delta":
				text := gjson.Get(data, "delta").String()
				if debug {
					slog.Info("SSE: delta event", "text", text, "hasText", text != "")
				}
				if text != "" {
					payload, _ := json.Marshal(map[string]any{"model": model, "output": text})
					n, _ := fmt.Fprintf(w, "data: %s\n\n", payload)
					totalBytes += n
					chunkCount++
					safeFlush(w)
					if debug {
						slog.Info("SSE: sent chunk", "chunkCount", chunkCount, "bytes", n)
					}
				}
			case "response.output_text.done", "response.done", "response.completed":
				if debug {
					slog.Debug("OpenAIResponsesAPISSEToOpenAIRes done event", "eventType", eventType)
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
				safeFlush(w)
				if debug {
					slog.Debug("OpenAIResponsesAPISSEToOpenAIRes completed", "chunks", chunkCount, "bytes", totalBytes)
				}
				return nil
			case "response.error", "error":
				errMsg := gjson.Get(data, "error.message").String()
				if errMsg == "" {
					errMsg = data
				}
				if debug {
					slog.Debug("OpenAIResponsesAPISSEToOpenAIRes error event", "error", errMsg)
				}
				return fmt.Errorf("openai responses stream error: %s", errMsg)
			default:
				if debug {
					slog.Debug("OpenAIResponsesAPISSEToOpenAIRes unknown event type", "eventType", eventType, "data", data)
				}
			}
			eventType = ""
			continue
		}

		// 处理无 event 的简化格式（直接是 data: {...}）
		if gjson.Get(data, "output").Exists() {
			if debug {
				slog.Info("SSE: output field exists, passthrough", "data", data)
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			chunkCount++
			safeFlush(w)
		} else if data == "[DONE]" {
			if debug {
				slog.Info("SSE: [DONE] received")
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
			safeFlush(w)
			return nil
		} else if gjson.Get(data, "choices").Exists() {
			content := gjson.Get(data, "choices.0.delta.content").String()
			if debug {
				slog.Info("SSE: choices format", "content", content, "hasContent", content != "")
			}
			if content != "" {
				payload, _ := json.Marshal(map[string]any{"model": model, "output": content})
				fmt.Fprintf(w, "data: %s\n\n", payload)
				chunkCount++
				safeFlush(w)
			}
		} else {
			if debug {
				slog.Info("SSE: unrecognized data format", "data", data)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("openai responses stream read error: %w", err)
	}
	if debug {
		slog.Debug("OpenAIResponsesAPISSEToOpenAIRes stream ended", "chunks", chunkCount)
	}
	return nil
}

// OpenAIResponsesAPIToOpenAIRes 将 OpenAI Responses API 的非流式响应转换为简化的 OpenAI-Res 格式
func OpenAIResponsesAPIToOpenAIRes(raw []byte, model string) ([]byte, error) {
	// 检查是否已经是简化格式（避免重复转换）
	if gjson.GetBytes(raw, "output").Exists() && gjson.GetBytes(raw, "output").Type == gjson.String {
		return raw, nil
	}

	id := gjson.GetBytes(raw, "id").String()
	created := gjson.GetBytes(raw, "created").Int()

	// 聚合所有 output 中的文本内容
	var textParts []string
	gjson.GetBytes(raw, "output").ForEach(func(_, item gjson.Result) bool {
		// 处理嵌套的 content 数组
		if content := item.Get("content"); content.Exists() && content.IsArray() {
			content.ForEach(func(_, c gjson.Result) bool {
				if c.Get("type").String() == "output_text" {
					if text := c.Get("text").String(); text != "" {
						textParts = append(textParts, text)
					}
				}
				return true
			})
		} else if item.Get("type").String() == "output_text" {
			// 处理简化格式的 output
			if text := item.Get("text").String(); text != "" {
				textParts = append(textParts, text)
			}
		}
		return true
	})

	output := strings.Join(textParts, "")
	if output == "" {
		return nil, fmt.Errorf("no text output found in response")
	}

	return json.Marshal(map[string]interface{}{
		"id":      id,
		"model":   model,
		"output":  output,
		"created": created,
	})
}

// DetectFormat 检测请求格式
func DetectFormat(raw []byte, fallback string) string {
	if gjson.GetBytes(raw, "input").Exists() {
		return consts.StyleOpenAIRes
	}
	msg := gjson.GetBytes(raw, "messages")
	model := strings.ToLower(gjson.GetBytes(raw, "model").String())
	// Anthropic 格式：messages[0].content 是数组 且 模型名包含 claude
	if msg.IsArray() && msg.Get("0.content").IsArray() && strings.Contains(model, "claude") {
		return consts.StyleAnthropic
	}
	return fallback
}

// OpenAIResTo OpenAI 将 OpenAI-Res 请求转换为 OpenAI 格式
func OpenAIResToOpenAI(raw []byte) ([]byte, error) {
	input := gjson.GetBytes(raw, "input").String()
	model := gjson.GetBytes(raw, "model").String()
	stream := gjson.GetBytes(raw, "stream").Bool()
	payload := map[string]any{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": input}},
	}
	if maxTokens := gjson.GetBytes(raw, "max_output_tokens").Int(); maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}
	if temp := gjson.GetBytes(raw, "temperature").Float(); temp != 0 {
		payload["temperature"] = temp
	}
	if stream {
		payload["stream"] = true
		payload["stream_options"] = map[string]bool{"include_usage": true}
	}
	return json.Marshal(payload)
}

// OpenAIToOpenAIRes 将 OpenAI 请求转换为 OpenAI-Res 格式
func OpenAIToOpenAIRes(raw []byte) ([]byte, error) {
	model := gjson.GetBytes(raw, "model").String()
	lastMsg := gjson.GetBytes(raw, "messages.#(role==\"user\")#.content").Array()
	var input string
	if len(lastMsg) > 0 {
		input = lastMsg[len(lastMsg)-1].String()
	}
	stream := gjson.GetBytes(raw, "stream").Bool()
	payload := map[string]any{"model": model, "input": input}
	if stream {
		payload["stream"] = true
	}
	return json.Marshal(payload)
}

// AnthropicToOpenAIRes 将 Anthropic 响应转换为 OpenAI-Res 格式
func AnthropicToOpenAIRes(raw []byte, model string) ([]byte, error) {
	content := gjson.GetBytes(raw, "content.0.text").String()
	return json.Marshal(map[string]interface{}{
		"id":      gjson.GetBytes(raw, "id").String(),
		"model":   model,
		"output":  content,
		"created": time.Now().Unix(),
	})
}

// OpenAIToOpenAIRes Response 将 OpenAI 响应转换为 OpenAI-Res 格式
func OpenAIRespToOpenAIRes(raw []byte, model string) ([]byte, error) {
	content := gjson.GetBytes(raw, "choices.0.message.content").String()
	return json.Marshal(map[string]interface{}{
		"id":      gjson.GetBytes(raw, "id").String(),
		"model":   model,
		"output":  content,
		"created": gjson.GetBytes(raw, "created").Int(),
	})
}

// OpenAIResToOpenAIResp 将 OpenAI-Res 响应转换为 OpenAI 格式
func OpenAIResToOpenAIResp(raw []byte, model string) ([]byte, error) {
	output := gjson.GetBytes(raw, "output").String()
	return json.Marshal(map[string]interface{}{
		"id":      gjson.GetBytes(raw, "id").String(),
		"object":  "chat.completion",
		"created": gjson.GetBytes(raw, "created").Int(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": output},
				"finish_reason": "stop",
			},
		},
	})
}

// CanConvertRequest 判断请求格式是否可以转换（客户端 -> Provider）
func CanConvertRequest(from, to string) bool {
	supported := map[string]map[string]bool{
		consts.StyleOpenAI: {
			consts.StyleOpenAIRes: true,
			consts.StyleAnthropic: true,
		},
		consts.StyleOpenAIRes: {
			consts.StyleOpenAI:    true,
			consts.StyleAnthropic: true,
		},
		consts.StyleAnthropic: {
			consts.StyleOpenAI:    true,
			consts.StyleOpenAIRes: true,
		},
	}
	return from == to || supported[from][to]
}

// CanConvertResponse 判断非流响应是否可以转换（Provider -> 客户端）
func CanConvertResponse(from, to string) bool {
	supported := map[string]map[string]bool{
		consts.StyleAnthropic: {
			consts.StyleOpenAI:    true,
			consts.StyleOpenAIRes: true,
		},
		consts.StyleOpenAI: {
			consts.StyleOpenAIRes: true,
			consts.StyleAnthropic: true,
		},
		consts.StyleOpenAIRes: {
			consts.StyleOpenAI:    true,
			consts.StyleAnthropic: true,
		},
	}
	return from == to || supported[from][to]
}

// CanConvertStream 判断流式响应是否可以转换（Provider -> 客户端）
func CanConvertStream(from, to string) bool {
	supported := map[string]map[string]bool{
		consts.StyleAnthropic: {
			consts.StyleOpenAI:    true,
			consts.StyleOpenAIRes: true,
		},
		consts.StyleOpenAI: {
			consts.StyleOpenAIRes: true,
			consts.StyleAnthropic: true,
		},
		consts.StyleOpenAIRes: {
			consts.StyleOpenAI:    true,
			consts.StyleAnthropic: true,
		},
	}
	return from == to || supported[from][to]
}

// ConvertRequest 转换请求格式
func ConvertRequest(raw []byte, from, to string) ([]byte, error) {
	provider := normalizeProviderStyle(to)

	// OpenAI-Res 请求可能需要格式规范化
	if from == consts.StyleOpenAIRes && to == consts.StyleOpenAIRes {
		return raw, nil
	}

	if from == provider {
		return raw, nil
	}
	switch {
	case from == consts.StyleOpenAI && provider == consts.StyleAnthropic:
		return OpenAIToAnthropic(raw)
	case from == consts.StyleOpenAI && provider == consts.StyleOpenAIRes:
		return OpenAIToOpenAIRes(raw)
	case from == consts.StyleOpenAIRes && provider == consts.StyleOpenAI:
		return OpenAIResToOpenAI(raw)
	case from == consts.StyleOpenAIRes && provider == consts.StyleAnthropic:
		converted, err := OpenAIResToOpenAI(raw)
		if err != nil {
			return nil, err
		}
		return OpenAIToAnthropic(converted)
	case from == consts.StyleAnthropic && provider == consts.StyleOpenAI:
		return AnthropicToOpenAIReq(raw)
	case from == consts.StyleAnthropic && provider == consts.StyleOpenAIRes:
		converted, err := AnthropicToOpenAIReq(raw)
		if err != nil {
			return nil, err
		}
		return OpenAIToOpenAIRes(converted)
	}
	return nil, fmt.Errorf("unsupported request convert: %s -> %s", from, to)
}

// ConvertResponse 转换响应格式（非流）
func ConvertResponse(raw []byte, from, to, model string) ([]byte, error) {
	provider := normalizeProviderStyle(from)

	// OpenAI-Res 需要强制转换，即使 from == to
	if from == consts.StyleOpenAIRes && to == consts.StyleOpenAIRes {
		return OpenAIResponsesAPIToOpenAIRes(raw, model)
	}

	// 自动检测：如果 Provider 声称是 openai 但实际返回 openai-res 格式
	if provider == consts.StyleOpenAI && to == consts.StyleOpenAIRes {
		if gjson.GetBytes(raw, "output").Exists() {
			// Provider 实际返回的是 openai-res 格式，直接转换
			return OpenAIResponsesAPIToOpenAIRes(raw, model)
		}
	}

	if provider == to {
		return raw, nil
	}
	switch {
	case provider == consts.StyleAnthropic && to == consts.StyleOpenAI:
		return AnthropicToOpenAI(raw, model)
	case provider == consts.StyleAnthropic && to == consts.StyleOpenAIRes:
		return AnthropicToOpenAIRes(raw, model)
	case provider == consts.StyleOpenAI && to == consts.StyleOpenAIRes:
		return OpenAIRespToOpenAIRes(raw, model)
	case provider == consts.StyleOpenAI && to == consts.StyleAnthropic:
		return OpenAIRespToAnthropic(raw, model)
	case from == consts.StyleOpenAIRes && to == consts.StyleOpenAI:
		return OpenAIResToOpenAIResp(raw, model)
	case from == consts.StyleOpenAIRes && to == consts.StyleAnthropic:
		return OpenAIResToAnthropic(raw, model)
	}
	return nil, fmt.Errorf("unsupported response convert: %s -> %s", from, to)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// 在读取前检查 context，确保客户端断连或取消时尽快退出转换
func (r contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}

// ConvertStream 转换流式响应
func ConvertStream(ctx context.Context, r io.Reader, w io.Writer, from, to, model string, debug bool) error {
	slog.Info("ConvertStream", "from", from, "to", to, "model", model)

	provider := normalizeProviderStyle(from)
	slog.Info("ConvertStream normalized", "provider", provider, "to", to)

	streamReader := r
	if ctx != nil {
		streamReader = contextReader{ctx: ctx, reader: r}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	// OpenAI-Res 需要强制转换，即使 from == to
	if from == consts.StyleOpenAIRes && to == consts.StyleOpenAIRes {
		slog.Info("ConvertStream: using OpenAIResponsesAPISSEToOpenAIRes (same format)")
		return OpenAIResponsesAPISSEToOpenAIRes(streamReader, w, model, debug)
	}

	if provider == to {
		slog.Info("ConvertStream: direct passthrough", "provider", provider)
		// Use buffered copy with immediate flush for each chunk
		buf := make([]byte, 4096)
		var totalBytes int64
		for {
			n, readErr := streamReader.Read(buf)
			if n > 0 {
				written, writeErr := w.Write(buf[:n])
				totalBytes += int64(written)
				if writeErr != nil {
					return fmt.Errorf("stream write failed after %d bytes: %w", totalBytes, writeErr)
				}
				// Flush immediately after each write
				safeFlush(w)
			}
			if readErr != nil {
				if readErr == io.EOF {
					if debug {
						slog.Debug("ConvertStream direct copy completed", "bytes", totalBytes)
					}
					return nil
				}
				return fmt.Errorf("stream copy failed after %d bytes: %w", totalBytes, readErr)
			}
		}
	}

	var err error
	switch {
	case from == consts.StyleOpenAIRes && to == consts.StyleOpenAI:
		err = OpenAIResSSEToOpenAI(streamReader, w, model)
	case from == consts.StyleOpenAIRes && to == consts.StyleAnthropic:
		err = OpenAIResSSEToAnthropic(streamReader, w, model)
	case provider == consts.StyleAnthropic && to == consts.StyleOpenAI:
		err = AnthropicSSEToOpenAI(streamReader, w, model)
	case provider == consts.StyleAnthropic && to == consts.StyleOpenAIRes:
		err = AnthropicSSEToOpenAIRes(streamReader, w, model, debug)
	case provider == consts.StyleOpenAI && to == consts.StyleOpenAIRes:
		slog.Info("ConvertStream: using OpenAISSEToOpenAIRes")
		err = OpenAISSEToOpenAIRes(streamReader, w, model)
	case provider == consts.StyleOpenAI && to == consts.StyleAnthropic:
		err = OpenAISSEToAnthropic(streamReader, w, model)
	default:
		err = fmt.Errorf("unsupported stream convert: %s -> %s (provider=%s)", from, to, provider)
	}

	if err != nil && debug {
		slog.Debug("ConvertStream completed with error", "error", err)
	}
	return err
}

// OpenAIRespToAnthropic 将 OpenAI 响应转换为 Anthropic 格式
func OpenAIRespToAnthropic(raw []byte, model string) ([]byte, error) {
	content := gjson.GetBytes(raw, "choices.0.message.content").String()
	id := gjson.GetBytes(raw, "id").String()
	return json.Marshal(map[string]any{
		"id":    id,
		"type":  "message",
		"model": model,
		"role":  "assistant",
		"content": []map[string]any{
			{"type": "text", "text": content},
		},
		"stop_reason": "end_turn",
	})
}

// OpenAIResToAnthropic 将 OpenAI-Res 响应转换为 Anthropic 格式
func OpenAIResToAnthropic(raw []byte, model string) ([]byte, error) {
	content := gjson.GetBytes(raw, "output").String()
	id := gjson.GetBytes(raw, "id").String()
	return json.Marshal(map[string]any{
		"id":    id,
		"type":  "message",
		"model": model,
		"role":  "assistant",
		"content": []map[string]any{
			{"type": "text", "text": content},
		},
		"stop_reason": "end_turn",
	})
}

// OpenAISSEToAnthropic 将 OpenAI SSE 流转换为 Anthropic SSE 格式
func OpenAISSEToAnthropic(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	start := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    msgID,
			"model": model,
			"type":  "message",
			"role":  "assistant",
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
		},
	}
	blockStart := map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}
	for _, evt := range []map[string]any{start, blockStart} {
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: %s\n", evt["type"])
		fmt.Fprintf(w, "data: %s\n\n", data)
		safeFlush(w)
	}
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "data: ")
		if line == "" || line == scanner.Text() {
			continue
		}
		if line == "[DONE]" {
			break
		}
		text := gjson.Get(line, "choices.0.delta.content").String()
		if text == "" {
			continue
		}
		evt := map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		}
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: content_block_delta\n")
		fmt.Fprintf(w, "data: %s\n\n", data)
		safeFlush(w)
	}
	for _, evt := range []map[string]any{{"type": "content_block_stop", "index": 0}, {"type": "message_stop", "stop_reason": "end_turn", "id": msgID}} {
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: %s\n", evt["type"])
		fmt.Fprintf(w, "data: %s\n\n", data)
		safeFlush(w)
	}
	return scanner.Err()
}

// OpenAIResSSEToAnthropic 将 OpenAI-Res SSE 流转换为 Anthropic SSE 格式
func OpenAIResSSEToAnthropic(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	start := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    msgID,
			"model": model,
			"type":  "message",
			"role":  "assistant",
			"usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
		},
	}
	blockStart := map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}
	for _, evt := range []map[string]any{start, blockStart} {
		data, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: %s\n", evt["type"])
		fmt.Fprintf(w, "data: %s\n\n", data)
		safeFlush(w)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			continue
		}
		dataLine := strings.TrimPrefix(line, "data: ")
		if dataLine == "" || dataLine == line {
			continue
		}
		if dataLine == "[DONE]" {
			break
		}
		text := gjson.Get(dataLine, "output").String()
		if text == "" {
			continue
		}
		evt := map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		}
		payload, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: content_block_delta\n")
		fmt.Fprintf(w, "data: %s\n\n", payload)
		safeFlush(w)
	}
	for _, evt := range []map[string]any{{"type": "content_block_stop", "index": 0}, {"type": "message_stop", "stop_reason": "end_turn", "id": msgID}} {
		payload, _ := json.Marshal(evt)
		fmt.Fprintf(w, "event: %s\n", evt["type"])
		fmt.Fprintf(w, "data: %s\n\n", payload)
		safeFlush(w)
	}
	return scanner.Err()
}

// CanConvert 判断是否可以在两种格式间转换（双向检查）
func CanConvert(from, to string) bool {
	return CanConvertRequest(from, to) && CanConvertResponse(to, from) && CanConvertStream(to, from)
}

// OpenAISSEToOpenAIRes 将 OpenAI SSE 流转换为 OpenAI-Res SSE 格式
func OpenAISSEToOpenAIRes(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var rawLines []string
	var chunkCount int
	for scanner.Scan() {
		text := scanner.Text()
		rawLines = append(rawLines, text)
		line := strings.TrimPrefix(text, "data: ")
		if line == "" || line == text {
			continue
		}
		if line == "[DONE]" {
			fmt.Fprint(w, "data: [DONE]\n\n")
			safeFlush(w)
			return nil
		}
		content := gjson.Get(line, "choices.0.delta.content").String()
		if content != "" {
			payload, _ := json.Marshal(map[string]any{"model": model, "output": content})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			safeFlush(w)
			chunkCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if chunkCount == 0 && len(rawLines) > 0 {
		raw := strings.TrimSpace(strings.Join(rawLines, "\n"))
		if raw != "" {
			if payload, convErr := OpenAIRespToOpenAIRes([]byte(raw), model); convErr == nil {
				fmt.Fprintf(w, "data: %s\n\n", payload)
				fmt.Fprint(w, "data: [DONE]\n\n")
				safeFlush(w)
				return nil
			}
		}
		return fmt.Errorf("openai stream ended without data")
	}
	return nil
}

// OpenAIResSSEToOpenAI 将 OpenAI-Res SSE 流转换为 OpenAI SSE 格式
func OpenAIResSSEToOpenAI(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
	created := time.Now().Unix()

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == line {
			continue
		}
		if data == "[DONE]" {
			fmt.Fprint(w, "data: [DONE]\n\n")
			safeFlush(w)
			return nil
		}
		content := gjson.Get(data, "output").String()
		if content != "" {
			chunk := map[string]any{
				"id":      chunkID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]string{"content": content},
					"finish_reason": nil,
				}},
			}
			payload, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", payload)
			safeFlush(w)
		}
	}
	return scanner.Err()
}
