package formatx

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/atopos31/llmio/consts"
	"github.com/tidwall/gjson"
)

// OpenAIToAnthropic 将 OpenAI 请求转换为 Anthropic 格式
func OpenAIToAnthropic(raw []byte) ([]byte, error) {
	var req struct {
		Model       string `json:"model"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens   int     `json:"max_tokens,omitempty"`
		Temperature float64 `json:"temperature,omitempty"`
		Stream      bool    `json:"stream,omitempty"`
	}

	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}

	// 提取 system 消息
	var system string
	var messages []map[string]interface{}

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			system = msg.Content
		} else {
			messages = append(messages, map[string]interface{}{
				"role": msg.Role,
				"content": []map[string]string{
					{"type": "text", "text": msg.Content},
				},
			})
		}
	}

	anthReq := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   req.Stream,
	}

	if system != "" {
		anthReq["system"] = system
	}
	if req.MaxTokens > 0 {
		anthReq["max_tokens"] = req.MaxTokens
	}
	if req.Temperature > 0 {
		anthReq["temperature"] = req.Temperature
	}

	return json.Marshal(anthReq)
}

// AnthropicToOpenAIReq 将 Anthropic 请求转换为 OpenAI 格式
func AnthropicToOpenAIReq(raw []byte) ([]byte, error) {
	system := gjson.GetBytes(raw, "system").String()
	model := gjson.GetBytes(raw, "model").String()
	stream := gjson.GetBytes(raw, "stream").Bool()

	var messages []map[string]string
	if system != "" {
		messages = append(messages, map[string]string{"role": "system", "content": system})
	}

	gjson.GetBytes(raw, "messages").ForEach(func(_, msg gjson.Result) bool {
		role := msg.Get("role").String()
		var content string
		if msg.Get("content").IsArray() {
			content = msg.Get("content.0.text").String()
		} else {
			content = msg.Get("content").String()
		}
		messages = append(messages, map[string]string{"role": role, "content": content})
		return true
	})

	return json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   stream,
	})
}

// AnthropicToOpenAI 将 Anthropic 响应转换为 OpenAI 格式
func AnthropicToOpenAI(raw []byte, model string) ([]byte, error) {
	var resp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}

	var content string
	if len(resp.Content) > 0 {
		content = resp.Content[0].Text
	}

	openaiResp := map[string]interface{}{
		"id":      resp.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     resp.Usage.InputTokens,
			"completion_tokens": resp.Usage.OutputTokens,
			"total_tokens":      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	return json.Marshal(openaiResp)
}

// AnthropicSSEToOpenAI 将 Anthropic SSE 流转换为 OpenAI SSE 格式
func AnthropicSSEToOpenAI(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string
	chunkID := fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
	created := time.Now().Unix()
	firstChunk := true

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
				var delta struct {
					Delta struct {
						Text string `json:"text"`
					} `json:"delta"`
				}
				if err := json.Unmarshal([]byte(data), &delta); err != nil {
					return fmt.Errorf("anthropic stream decode error: %w", err)
				}
				if delta.Delta.Text != "" {
					if firstChunk {
						roleChunk := map[string]interface{}{
							"id":      chunkID,
							"object":  "chat.completion.chunk",
							"created": created,
							"model":   model,
							"choices": []map[string]interface{}{
								{
									"index":         0,
									"delta":         map[string]string{"role": "assistant"},
									"finish_reason": nil,
								},
							},
						}
						roleBytes, _ := json.Marshal(roleChunk)
						fmt.Fprintf(w, "data: %s\n\n", roleBytes)
						firstChunk = false
					}

					chunk := map[string]interface{}{
						"id":      chunkID,
						"object":  "chat.completion.chunk",
						"created": created,
						"model":   model,
						"choices": []map[string]interface{}{
							{
								"index":         0,
								"delta":         map[string]string{"content": delta.Delta.Text},
								"finish_reason": nil,
							},
						},
					}
					chunkBytes, _ := json.Marshal(chunk)
					fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
				}

			case "message_stop":
				finalChunk := map[string]interface{}{
					"id":      chunkID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   model,
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         map[string]interface{}{},
							"finish_reason": "stop",
						},
					},
				}
				finalBytes, _ := json.Marshal(finalChunk)
				fmt.Fprintf(w, "data: %s\n\n", finalBytes)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				return nil

			case "error":
				return fmt.Errorf("anthropic stream error: %s", data)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("anthropic stream read error: %w", err)
	}
	return fmt.Errorf("anthropic stream closed without stop event")
}

// AnthropicSSEToOpenAIRes 将 Anthropic SSE 流转换为 OpenAI-Res SSE 格式
func AnthropicSSEToOpenAIRes(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string

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
				if text != "" {
					chunk := map[string]interface{}{
						"model":  model,
						"output": text,
					}
					chunkBytes, _ := json.Marshal(chunk)
					fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
					if flusher, ok := w.(http.Flusher); ok {
						flusher.Flush()
					}
				}

			case "message_stop":
				fmt.Fprintf(w, "data: [DONE]\n\n")
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				return nil

			case "error":
				return fmt.Errorf("anthropic stream error: %s", data)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("anthropic stream read error: %w", err)
	}
	return fmt.Errorf("anthropic stream closed without stop event")
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
	return json.Marshal(map[string]interface{}{
		"model":    model,
		"messages": []map[string]string{{"role": "user", "content": input}},
	})
}

// OpenAIToOpenAIRes 将 OpenAI 请求转换为 OpenAI-Res 格式
func OpenAIToOpenAIRes(raw []byte) ([]byte, error) {
	model := gjson.GetBytes(raw, "model").String()
	lastMsg := gjson.GetBytes(raw, "messages.#(role==\"user\")#.content").Array()
	var input string
	if len(lastMsg) > 0 {
		input = lastMsg[len(lastMsg)-1].String()
	}
	return json.Marshal(map[string]string{"model": model, "input": input})
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

// CanConvert 判断是否支持格式转换
func CanConvert(from, to string) bool {
	if from == to {
		return true
	}
	validFormats := map[string]bool{
		consts.StyleOpenAI:    true,
		consts.StyleAnthropic: true,
		consts.StyleOpenAIRes: true,
	}
	return validFormats[from] && validFormats[to]
}

// ConvertRequest 转换请求格式
func ConvertRequest(raw []byte, from, to string) ([]byte, error) {
	if from == to {
		return raw, nil
	}
	switch {
	case from == consts.StyleOpenAI && to == consts.StyleAnthropic:
		return OpenAIToAnthropic(raw)
	case from == consts.StyleOpenAI && to == consts.StyleOpenAIRes:
		return OpenAIToOpenAIRes(raw)
	case from == consts.StyleOpenAIRes && to == consts.StyleOpenAI:
		return OpenAIResToOpenAI(raw)
	case from == consts.StyleOpenAIRes && to == consts.StyleAnthropic:
		converted, err := OpenAIResToOpenAI(raw)
		if err != nil {
			return nil, err
		}
		return OpenAIToAnthropic(converted)
	case from == consts.StyleAnthropic && to == consts.StyleOpenAI:
		return AnthropicToOpenAIReq(raw)
	case from == consts.StyleAnthropic && to == consts.StyleOpenAIRes:
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
	if from == to {
		return raw, nil
	}
	switch {
	case from == consts.StyleAnthropic && to == consts.StyleOpenAI:
		return AnthropicToOpenAI(raw, model)
	case from == consts.StyleAnthropic && to == consts.StyleOpenAIRes:
		return AnthropicToOpenAIRes(raw, model)
	case from == consts.StyleOpenAI && to == consts.StyleOpenAIRes:
		return OpenAIRespToOpenAIRes(raw, model)
	case from == consts.StyleOpenAI && to == consts.StyleAnthropic:
		return raw, nil // 不支持 OpenAI 响应转 Anthropic
	case from == consts.StyleOpenAIRes && to == consts.StyleOpenAI:
		return OpenAIResToOpenAIResp(raw, model)
	case from == consts.StyleOpenAIRes && to == consts.StyleAnthropic:
		return raw, nil // 不支持 OpenAI-Res 响应转 Anthropic
	}
	return nil, fmt.Errorf("unsupported response convert: %s -> %s", from, to)
}

// ConvertStream 转换流式响应
func ConvertStream(ctx context.Context, r io.Reader, w io.Writer, from, to, model string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if from == to {
		_, err := io.Copy(w, r)
		return err
	}
	switch {
	case from == consts.StyleAnthropic && to == consts.StyleOpenAI:
		return AnthropicSSEToOpenAI(r, w, model)
	case from == consts.StyleAnthropic && to == consts.StyleOpenAIRes:
		return AnthropicSSEToOpenAIRes(r, w, model)
	case from == consts.StyleOpenAI && to == consts.StyleOpenAIRes:
		return OpenAISSEToOpenAIRes(r, w, model)
	case from == consts.StyleOpenAIRes && to == consts.StyleOpenAI:
		return OpenAIResSSEToOpenAI(r, w, model)
	}
	_, err := io.Copy(w, r)
	return err
}

// OpenAISSEToOpenAIRes 将 OpenAI SSE 流转换为 OpenAI-Res SSE 格式
func OpenAISSEToOpenAIRes(r io.Reader, w io.Writer, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "data: ")
		if line == "" || line == scanner.Text() {
			continue
		}
		if line == "[DONE]" {
			fmt.Fprint(w, "data: [DONE]\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return nil
		}
		content := gjson.Get(line, "choices.0.delta.content").String()
		if content != "" {
			payload, _ := json.Marshal(map[string]any{"model": model, "output": content})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
	return scanner.Err()
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
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
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
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
	return scanner.Err()
}
