package adapters

import (
	"encoding/json"
	"net/http"

	"github.com/atopos31/llmio/providers/unified"
)

// OpenAIAdapter OpenAI协议适配器
type OpenAIAdapter struct{}

// OpenAIRequest OpenAI请求格式
type OpenAIRequest struct {
	Model       string                   `json:"model"`
	Messages    []OpenAIMessage          `json:"messages"`
	MaxTokens   *int                     `json:"max_tokens,omitempty"`
	Temperature *float64                 `json:"temperature,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
	Stop        interface{}              `json:"stop,omitempty"`
	Stream      bool                     `json:"stream,omitempty"`
	Tools       []OpenAITool             `json:"tools,omitempty"`
	Metadata    map[string]interface{}   `json:"metadata,omitempty"`
}

type OpenAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []OpenAIContent
	Name    string      `json:"name,omitempty"`
}

type OpenAIContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type OpenAITool struct {
	Type     string                 `json:"type"`
	Function OpenAIFunction         `json:"function"`
}

type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

func NewOpenAIAdapter() *OpenAIAdapter {
	return &OpenAIAdapter{}
}

func (a *OpenAIAdapter) Protocol() string {
	return "openai"
}

func (a *OpenAIAdapter) ParseRequest(rawBody []byte) (*unified.CanonicalRequest, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return nil, err
	}

	canonical := &unified.CanonicalRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Metadata:    req.Metadata,
	}

	// 转换消息
	for _, msg := range req.Messages {
		canonicalMsg := unified.CanonicalMessage{
			Role: msg.Role,
			Name: msg.Name,
		}

		// 处理content字段（可能是string或[]OpenAIContent）
		switch content := msg.Content.(type) {
		case string:
			canonicalMsg.Content = []unified.CanonicalContent{
				{Type: "text", Text: content},
			}
		case []interface{}:
			for _, c := range content {
				if contentMap, ok := c.(map[string]interface{}); ok {
					canonicalContent := unified.CanonicalContent{
						Type: contentMap["type"].(string),
					}
					if text, exists := contentMap["text"]; exists {
						canonicalContent.Text = text.(string)
					}
					canonicalMsg.Content = append(canonicalMsg.Content, canonicalContent)
				}
			}
		}

		canonical.Messages = append(canonical.Messages, canonicalMsg)
	}

	// 转换工具
	for _, tool := range req.Tools {
		canonical.Tools = append(canonical.Tools, unified.CanonicalTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}

	// 处理stop参数
	if req.Stop != nil {
		switch stop := req.Stop.(type) {
		case string:
			canonical.Stop = []string{stop}
		case []interface{}:
			for _, s := range stop {
				if str, ok := s.(string); ok {
					canonical.Stop = append(canonical.Stop, str)
				}
			}
		}
	}

	return canonical, nil
}

func (a *OpenAIAdapter) BuildUpstreamRequest(canonical *unified.CanonicalRequest) ([]byte, http.Header, error) {
	// 这里将canonical格式转换为上游Provider（如Anthropic）的格式
	// 暂时返回原始格式，后续实现具体转换逻辑
	return json.Marshal(canonical)
}

func (a *OpenAIAdapter) ParseUpstreamResponse(resp *http.Response) (*unified.CanonicalResponse, error) {
	// 解析上游响应为canonical格式
	// 暂时返回空实现
	return &unified.CanonicalResponse{}, nil
}

func (a *OpenAIAdapter) BuildResponse(canonical *unified.CanonicalResponse) ([]byte, http.Header, error) {
	// 将canonical格式转换为OpenAI响应格式
	// 暂时返回空实现
	return json.Marshal(canonical)
}