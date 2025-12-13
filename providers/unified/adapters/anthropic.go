package adapters

import (
	"encoding/json"
	"net/http"
)

// AnthropicAdapter Anthropic协议适配器
type AnthropicAdapter struct{}

// AnthropicRequest Anthropic请求格式
type AnthropicRequest struct {
	Model       string                 `json:"model"`
	Messages    []AnthropicMessage     `json:"messages"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature *float64               `json:"temperature,omitempty"`
	TopP        *float64               `json:"top_p,omitempty"`
	TopK        *int                   `json:"top_k,omitempty"`
	Stop        []string               `json:"stop_sequences,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Tools       []AnthropicTool        `json:"tools,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type AnthropicMessage struct {
	Role    string             `json:"role"`
	Content []AnthropicContent `json:"content"`
}

type AnthropicContent struct {
	Type string                 `json:"type"`
	Text string                 `json:"text,omitempty"`
	Data map[string]interface{} `json:",inline"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

func NewAnthropicAdapter() *AnthropicAdapter {
	return &AnthropicAdapter{}
}

func (a *AnthropicAdapter) Protocol() string {
	return "anthropic"
}

func (a *AnthropicAdapter) ParseRequest(rawBody []byte) (*unified.CanonicalRequest, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return nil, err
	}

	canonical := &unified.CanonicalRequest{
		Model:       req.Model,
		MaxTokens:   &req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		TopK:        req.TopK,
		Stop:        req.Stop,
		Stream:      req.Stream,
		Metadata:    req.Metadata,
	}

	// 转换消息
	for _, msg := range req.Messages {
		canonicalMsg := unified.CanonicalMessage{
			Role: msg.Role,
		}

		for _, content := range msg.Content {
			canonicalMsg.Content = append(canonicalMsg.Content, unified.CanonicalContent{
				Type: content.Type,
				Text: content.Text,
				Data: content.Data,
			})
		}

		canonical.Messages = append(canonical.Messages, canonicalMsg)
	}

	// 转换工具
	for _, tool := range req.Tools {
		canonical.Tools = append(canonical.Tools, unified.CanonicalTool{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}

	return canonical, nil
}

func (a *AnthropicAdapter) BuildUpstreamRequest(canonical *unified.CanonicalRequest) ([]byte, http.Header, error) {
	// 将canonical格式转换为上游Provider格式
	// 暂时返回原始格式
	return json.Marshal(canonical)
}

func (a *AnthropicAdapter) ParseUpstreamResponse(resp *http.Response) (*unified.CanonicalResponse, error) {
	// 解析上游响应为canonical格式
	return &unified.CanonicalResponse{}, nil
}

func (a *AnthropicAdapter) BuildResponse(canonical *unified.CanonicalResponse) ([]byte, http.Header, error) {
	// 将canonical格式转换为Anthropic响应格式
	return json.Marshal(canonical)
}