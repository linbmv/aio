package transformers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/linbmv/aio/providers/unified"
)

// OpenAIResponseTransformer OpenAI响应转换器
type OpenAIResponseTransformer struct{}

// NewOpenAIResponseTransformer 创建OpenAI响应转换器
func NewOpenAIResponseTransformer() *OpenAIResponseTransformer {
	return &OpenAIResponseTransformer{}
}

// Protocol 获取协议名称
func (t *OpenAIResponseTransformer) Protocol() string {
	return "openai"
}

// Transform 将内部统一格式转换为OpenAI协议响应
func (t *OpenAIResponseTransformer) Transform(ctx context.Context, canonical *unified.CanonicalResponse, originalProtocol string) (*http.Response, error) {
	// 构建OpenAI响应格式
	openaiResp := OpenAIClientResponse{
		ID:      canonical.ID,
		Object:  "chat.completion",
		Created: canonical.Created.Unix(),
		Model:   canonical.Model,
		Usage: OpenAIClientUsage{
			PromptTokens:     canonical.Usage.PromptTokens,
			CompletionTokens: canonical.Usage.CompletionTokens,
			TotalTokens:      canonical.Usage.TotalTokens,
		},
	}

	// 转换选择
	openaiResp.Choices = make([]OpenAIClientChoice, len(canonical.Choices))
	for i, choice := range canonical.Choices {
		openaiChoice := OpenAIClientChoice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
			Message: OpenAIClientMessage{
				Role: choice.Message.Role,
			},
		}

		// 处理消息内容
		if len(choice.Message.Content) == 1 && choice.Message.Content[0].Type == "text" {
			// 简单文本消息
			openaiChoice.Message.Content = choice.Message.Content[0].Text
		} else {
			// 复杂内容数组
			contentArray := make([]map[string]interface{}, len(choice.Message.Content))
			for j, content := range choice.Message.Content {
				contentItem := map[string]interface{}{
					"type": content.Type,
				}

				if content.Text != "" {
					contentItem["text"] = content.Text
				}

				// 添加其他数据
				for k, v := range content.Data {
					contentItem[k] = v
				}

				contentArray[j] = contentItem
			}
			openaiChoice.Message.Content = contentArray
		}

		openaiResp.Choices[i] = openaiChoice
	}

	// 序列化响应
	respBody, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI response: %w", err)
	}

	// 创建HTTP响应
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("X-Transformed-From", originalProtocol)

	return resp, nil
}

// AnthropicResponseTransformer Anthropic响应转换器
type AnthropicResponseTransformer struct{}

// NewAnthropicResponseTransformer 创建Anthropic响应转换器
func NewAnthropicResponseTransformer() *AnthropicResponseTransformer {
	return &AnthropicResponseTransformer{}
}

// Protocol 获取协议名称
func (t *AnthropicResponseTransformer) Protocol() string {
	return "anthropic"
}

// Transform 将内部统一格式转换为Anthropic协议响应
func (t *AnthropicResponseTransformer) Transform(ctx context.Context, canonical *unified.CanonicalResponse, originalProtocol string) (*http.Response, error) {
	// 构建Anthropic响应格式
	anthropicResp := AnthropicClientResponse{
		ID:    canonical.ID,
		Type:  "message",
		Model: canonical.Model,
		Usage: AnthropicClientUsage{
			InputTokens:  canonical.Usage.PromptTokens,
			OutputTokens: canonical.Usage.CompletionTokens,
		},
	}

	// Anthropic只有一个选择，取第一个
	if len(canonical.Choices) > 0 {
		choice := canonical.Choices[0]
		anthropicResp.Role = choice.Message.Role
		anthropicResp.StopReason = choice.FinishReason

		// 转换内容
		anthropicResp.Content = make([]AnthropicClientContent, len(choice.Message.Content))
		for i, content := range choice.Message.Content {
			anthropicContent := AnthropicClientContent{
				Type: content.Type,
				Text: content.Text,
			}

			// 处理特殊内容类型
			switch content.Type {
			case "tool_use":
				if id, exists := content.Data["id"]; exists {
					anthropicContent.ID = id.(string)
				}
				if name, exists := content.Data["name"]; exists {
					anthropicContent.Name = name.(string)
				}
				if input, exists := content.Data["input"]; exists {
					anthropicContent.Input = input.(map[string]interface{})
				}
			}

			anthropicResp.Content[i] = anthropicContent
		}
	}

	// 从元数据中获取stop信息
	if canonical.Metadata != nil {
		if stopSequence, exists := canonical.Metadata["stop_sequence"]; exists {
			if seqStr, ok := stopSequence.(string); ok {
				anthropicResp.StopSequence = seqStr
			}
		}
	}

	// 序列化响应
	respBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic response: %w", err)
	}

	// 创建HTTP响应
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(respBody)),
	}

	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("X-Transformed-From", originalProtocol)

	return resp, nil
}

// OpenAI客户端响应结构体
type OpenAIClientResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []OpenAIClientChoice `json:"choices"`
	Usage   OpenAIClientUsage    `json:"usage"`
}

type OpenAIClientChoice struct {
	Index        int                 `json:"index"`
	Message      OpenAIClientMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
}

type OpenAIClientMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type OpenAIClientUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Anthropic客户端响应结构体
type AnthropicClientResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Model        string                   `json:"model"`
	Content      []AnthropicClientContent `json:"content"`
	StopReason   string                   `json:"stop_reason"`
	StopSequence string                   `json:"stop_sequence,omitempty"`
	Usage        AnthropicClientUsage     `json:"usage"`
}

type AnthropicClientContent struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

type AnthropicClientUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
