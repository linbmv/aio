package transformers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/linbmv/aio/providers/unified"
)

// OpenAIOutboundTransformer OpenAI出站协议转换器
type OpenAIOutboundTransformer struct{}

// NewOpenAIOutboundTransformer 创建OpenAI出站转换器
func NewOpenAIOutboundTransformer() *OpenAIOutboundTransformer {
	return &OpenAIOutboundTransformer{}
}

// Protocol 获取协议名称
func (t *OpenAIOutboundTransformer) Protocol() string {
	return "openai"
}

// CanHandle 检测是否支持该Provider
func (t *OpenAIOutboundTransformer) CanHandle(providerType string) bool {
	return strings.ToLower(providerType) == "openai" ||
		   strings.Contains(strings.ToLower(providerType), "openai")
}

// Transform 将内部统一格式转换为OpenAI Provider请求
func (t *OpenAIOutboundTransformer) Transform(ctx context.Context, canonical *unified.CanonicalRequest, endpoint string) (*http.Request, error) {
	// 构建OpenAI请求格式
	openaiReq := OpenAIUpstreamRequest{
		Model:       canonical.Model,
		MaxTokens:   canonical.MaxTokens,
		Temperature: canonical.Temperature,
		TopP:        canonical.TopP,
		Stop:        canonical.Stop,
		Stream:      canonical.Stream,
	}

	// 转换消息
	openaiReq.Messages = make([]OpenAIUpstreamMessage, len(canonical.Messages))
	for i, msg := range canonical.Messages {
		openaiMsg := OpenAIUpstreamMessage{
			Role: msg.Role,
			Name: msg.Name,
		}

		// 处理消息内容
		if len(msg.Content) == 1 && msg.Content[0].Type == "text" {
			// 简单文本消息
			openaiMsg.Content = msg.Content[0].Text
		} else {
			// 复杂内容数组
			contentArray := make([]map[string]interface{}, len(msg.Content))
			for j, content := range msg.Content {
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
			openaiMsg.Content = contentArray
		}

		openaiReq.Messages[i] = openaiMsg
	}

	// 转换工具
	if canonical.Tools != nil {
		openaiReq.Tools = make([]OpenAIUpstreamTool, len(canonical.Tools))
		for i, tool := range canonical.Tools {
			openaiReq.Tools[i] = OpenAIUpstreamTool{
				Type: "function",
				Function: OpenAIUpstreamFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				},
			}
		}
	}

	// 序列化请求
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// 设置头部
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LLMIO-Unified-Gateway/1.0")

	// 从元数据中恢复原始头部
	if canonical.Metadata != nil {
		if auth, exists := canonical.Metadata["authorization"]; exists {
			if authStr, ok := auth.(string); ok {
				req.Header.Set("Authorization", authStr)
			}
		}
	}

	return req, nil
}

// ParseResponse 将OpenAI响应转换为内部统一格式
func (t *OpenAIOutboundTransformer) ParseResponse(ctx context.Context, resp *http.Response) (*unified.CanonicalResponse, error) {
	var openaiResp OpenAIUpstreamResponse
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode OpenAI response: %w", err)
	}

	// 转换为统一格式
	canonical := &unified.CanonicalResponse{
		ID:      openaiResp.ID,
		Model:   openaiResp.Model,
		Created: openaiResp.Created,
		Usage: unified.CanonicalUsage{
			PromptTokens:     openaiResp.Usage.PromptTokens,
			CompletionTokens: openaiResp.Usage.CompletionTokens,
			TotalTokens:      openaiResp.Usage.TotalTokens,
			Metadata:         make(map[string]interface{}),
		},
		Metadata: make(map[string]interface{}),
	}

	// 转换选择
	canonical.Choices = make([]unified.CanonicalChoice, len(openaiResp.Choices))
	for i, choice := range openaiResp.Choices {
		canonicalChoice := unified.CanonicalChoice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
			Metadata:     make(map[string]interface{}),
		}

		// 转换消息
		canonicalChoice.Message = unified.CanonicalMessage{
			Role:     choice.Message.Role,
			Metadata: make(map[string]interface{}),
		}

		// 处理消息内容
		if choice.Message.Content != nil {
			switch content := choice.Message.Content.(type) {
			case string:
				canonicalChoice.Message.Content = []unified.CanonicalContent{
					{
						Type: "text",
						Text: content,
					},
				}
			case []interface{}:
				canonicalChoice.Message.Content = make([]unified.CanonicalContent, len(content))
				for j, item := range content {
					if itemMap, ok := item.(map[string]interface{}); ok {
						canonicalContent := unified.CanonicalContent{
							Data: make(map[string]interface{}),
						}

						if contentType, exists := itemMap["type"]; exists {
							canonicalContent.Type = fmt.Sprintf("%v", contentType)
						}

						if text, exists := itemMap["text"]; exists {
							canonicalContent.Text = fmt.Sprintf("%v", text)
						}

						// 复制其他数据
						for k, v := range itemMap {
							if k != "type" && k != "text" {
								canonicalContent.Data[k] = v
							}
						}

						canonicalChoice.Message.Content[j] = canonicalContent
					}
				}
			}
		}

		canonical.Choices[i] = canonicalChoice
	}

	canonical.Metadata["original_protocol"] = "openai"
	return canonical, nil
}

// OpenAI上游请求结构体
type OpenAIUpstreamRequest struct {
	Model       string                    `json:"model"`
	Messages    []OpenAIUpstreamMessage   `json:"messages"`
	MaxTokens   *int                      `json:"max_tokens,omitempty"`
	Temperature *float64                  `json:"temperature,omitempty"`
	TopP        *float64                  `json:"top_p,omitempty"`
	Stop        []string                  `json:"stop,omitempty"`
	Stream      bool                      `json:"stream,omitempty"`
	Tools       []OpenAIUpstreamTool      `json:"tools,omitempty"`
}

type OpenAIUpstreamMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content,omitempty"`
	Name    string      `json:"name,omitempty"`
}

type OpenAIUpstreamTool struct {
	Type     string                `json:"type"`
	Function OpenAIUpstreamFunction `json:"function"`
}

type OpenAIUpstreamFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// OpenAI上游响应结构体
type OpenAIUpstreamResponse struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []OpenAIUpstreamChoice   `json:"choices"`
	Usage   OpenAIUpstreamUsage      `json:"usage"`
}

type OpenAIUpstreamChoice struct {
	Index        int                     `json:"index"`
	Message      OpenAIUpstreamMessage   `json:"message"`
	FinishReason string                  `json:"finish_reason"`
}

type OpenAIUpstreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}