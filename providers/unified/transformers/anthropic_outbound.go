package transformers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/linbmv/aio/providers/unified"
)

// AnthropicOutboundTransformer Anthropic出站协议转换器
type AnthropicOutboundTransformer struct{}

// NewAnthropicOutboundTransformer 创建Anthropic出站转换器
func NewAnthropicOutboundTransformer() *AnthropicOutboundTransformer {
	return &AnthropicOutboundTransformer{}
}

// Protocol 获取协议名称
func (t *AnthropicOutboundTransformer) Protocol() string {
	return "anthropic"
}

// CanHandle 检测是否支持该Provider
func (t *AnthropicOutboundTransformer) CanHandle(providerType string) bool {
	return strings.ToLower(providerType) == "anthropic" ||
		   strings.Contains(strings.ToLower(providerType), "anthropic") ||
		   strings.Contains(strings.ToLower(providerType), "claude")
}

// Transform 将内部统一格式转换为Anthropic Provider请求
func (t *AnthropicOutboundTransformer) Transform(ctx context.Context, canonical *unified.CanonicalRequest, endpoint string) (*http.Request, error) {
	// 构建Anthropic请求格式
	anthropicReq := AnthropicUpstreamRequest{
		Model:         canonical.Model,
		MaxTokens:     *canonical.MaxTokens, // Anthropic要求必填
		Temperature:   canonical.Temperature,
		TopP:          canonical.TopP,
		TopK:          canonical.TopK,
		StopSequences: canonical.Stop,
		Stream:        canonical.Stream,
	}

	// 分离system消息和其他消息
	var systemMessage string
	var messages []unified.CanonicalMessage

	for _, msg := range canonical.Messages {
		if msg.Role == "system" {
			// 合并system消息
			for _, content := range msg.Content {
				if content.Type == "text" {
					if systemMessage != "" {
						systemMessage += "\n\n"
					}
					systemMessage += content.Text
				}
			}
		} else {
			messages = append(messages, msg)
		}
	}

	anthropicReq.System = systemMessage

	// 转换消息
	anthropicReq.Messages = make([]AnthropicUpstreamMessage, len(messages))
	for i, msg := range messages {
		anthropicMsg := AnthropicUpstreamMessage{
			Role: msg.Role,
		}

		// 转换内容
		anthropicMsg.Content = make([]AnthropicUpstreamContent, len(msg.Content))
		for j, content := range msg.Content {
			anthropicContent := AnthropicUpstreamContent{
				Type: content.Type,
				Text: content.Text,
			}

			// 处理特殊内容类型
			switch content.Type {
			case "image":
				if source, exists := content.Data["source"]; exists {
					anthropicContent.Source = source.(map[string]interface{})
				}
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
			case "tool_result":
				if toolUseID, exists := content.Data["tool_use_id"]; exists {
					anthropicContent.ToolUseID = toolUseID.(string)
				}
				if isError, exists := content.Data["is_error"]; exists {
					anthropicContent.IsError = isError.(bool)
				}
			}

			anthropicMsg.Content[j] = anthropicContent
		}

		anthropicReq.Messages[i] = anthropicMsg
	}

	// 转换工具
	if canonical.Tools != nil {
		anthropicReq.Tools = make([]AnthropicUpstreamTool, len(canonical.Tools))
		for i, tool := range canonical.Tools {
			anthropicReq.Tools[i] = AnthropicUpstreamTool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: tool.Parameters,
			}
		}
	}

	// 序列化请求
	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Anthropic request: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// 设置头部
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LLMIO-Unified-Gateway/1.0")
	req.Header.Set("anthropic-version", "2023-06-01")

	// 从元数据中恢复原始头部
	if canonical.Metadata != nil {
		if apiKey, exists := canonical.Metadata["x_api_key"]; exists {
			if keyStr, ok := apiKey.(string); ok {
				req.Header.Set("x-api-key", keyStr)
			}
		}
	}

	return req, nil
}

// ParseResponse 将Anthropic响应转换为内部统一格式
func (t *AnthropicOutboundTransformer) ParseResponse(ctx context.Context, resp *http.Response) (*unified.CanonicalResponse, error) {
	var anthropicResp AnthropicUpstreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthropicResp); err != nil {
		return nil, fmt.Errorf("failed to decode Anthropic response: %w", err)
	}

	// 转换为统一格式
	canonical := &unified.CanonicalResponse{
		ID:      anthropicResp.ID,
		Model:   anthropicResp.Model,
		Created: time.Now(), // Anthropic不返回created时间
		Usage: unified.CanonicalUsage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
			Metadata:         make(map[string]interface{}),
		},
		Metadata: make(map[string]interface{}),
	}

	// Anthropic只有一个选择
	canonical.Choices = []unified.CanonicalChoice{
		{
			Index:        0,
			FinishReason: anthropicResp.StopReason,
			Message: unified.CanonicalMessage{
				Role:     anthropicResp.Role,
				Metadata: make(map[string]interface{}),
			},
			Metadata: make(map[string]interface{}),
		},
	}

	// 转换内容
	canonical.Choices[0].Message.Content = make([]unified.CanonicalContent, len(anthropicResp.Content))
	for i, content := range anthropicResp.Content {
		canonicalContent := unified.CanonicalContent{
			Type: content.Type,
			Text: content.Text,
			Data: make(map[string]interface{}),
		}

		// 处理特殊内容类型
		switch content.Type {
		case "tool_use":
			canonicalContent.Data["id"] = content.ID
			canonicalContent.Data["name"] = content.Name
			canonicalContent.Data["input"] = content.Input
		}

		canonical.Choices[0].Message.Content[i] = canonicalContent
	}

	canonical.Metadata["original_protocol"] = "anthropic"
	canonical.Metadata["stop_reason"] = anthropicResp.StopReason
	canonical.Metadata["stop_sequence"] = anthropicResp.StopSequence

	return canonical, nil
}

// Anthropic上游请求结构体
type AnthropicUpstreamRequest struct {
	Model         string                      `json:"model"`
	MaxTokens     int                         `json:"max_tokens"`
	Messages      []AnthropicUpstreamMessage  `json:"messages"`
	System        string                      `json:"system,omitempty"`
	Temperature   *float64                    `json:"temperature,omitempty"`
	TopP          *float64                    `json:"top_p,omitempty"`
	TopK          *int                        `json:"top_k,omitempty"`
	StopSequences []string                    `json:"stop_sequences,omitempty"`
	Stream        bool                        `json:"stream,omitempty"`
	Tools         []AnthropicUpstreamTool     `json:"tools,omitempty"`
}

type AnthropicUpstreamMessage struct {
	Role    string                      `json:"role"`
	Content []AnthropicUpstreamContent  `json:"content"`
}

type AnthropicUpstreamContent struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Source    map[string]interface{} `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
}

type AnthropicUpstreamTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// Anthropic上游响应结构体
type AnthropicUpstreamResponse struct {
	ID           string                      `json:"id"`
	Type         string                      `json:"type"`
	Role         string                      `json:"role"`
	Model        string                      `json:"model"`
	Content      []AnthropicUpstreamContent  `json:"content"`
	StopReason   string                      `json:"stop_reason"`
	StopSequence string                      `json:"stop_sequence,omitempty"`
	Usage        AnthropicUpstreamUsage      `json:"usage"`
}

type AnthropicUpstreamUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}