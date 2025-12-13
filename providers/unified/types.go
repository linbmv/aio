package unified

import (
	"context"
	"net/http"
	"time"
)

// CanonicalRequest 统一的内部请求格式
type CanonicalRequest struct {
	Model       string                 `json:"model"`
	Messages    []CanonicalMessage     `json:"messages"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Temperature *float64               `json:"temperature,omitempty"`
	TopP        *float64               `json:"top_p,omitempty"`
	TopK        *int                   `json:"top_k,omitempty"`
	Stop        []string               `json:"stop,omitempty"`
	Stream      bool                   `json:"stream"`
	Tools       []CanonicalTool        `json:"tools,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CanonicalMessage 统一的消息格式
type CanonicalMessage struct {
	Role    string                 `json:"role"`
	Content []CanonicalContent     `json:"content"`
	Name    string                 `json:"name,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CanonicalContent 统一的内容格式
type CanonicalContent struct {
	Type string                 `json:"type"` // text, image, tool_use, tool_result
	Text string                 `json:"text,omitempty"`
	Data map[string]interface{} `json:"data,omitempty"` // 扩展数据
}

// CanonicalTool 统一的工具格式
type CanonicalTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// CanonicalResponse 统一的响应格式
type CanonicalResponse struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []CanonicalChoice      `json:"choices"`
	Usage   CanonicalUsage         `json:"usage"`
	Created time.Time              `json:"created"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// CanonicalChoice 统一的选择格式
type CanonicalChoice struct {
	Index        int                    `json:"index"`
	Message      CanonicalMessage       `json:"message"`
	FinishReason string                 `json:"finish_reason"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// CanonicalUsage 统一的使用量格式
type CanonicalUsage struct {
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	TotalTokens      int                    `json:"total_tokens"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// ProtocolAdapter 协议适配器接口
type ProtocolAdapter interface {
	// 将外部协议请求转换为内部统一格式
	ParseRequest(rawBody []byte) (*CanonicalRequest, error)

	// 将内部统一格式转换为上游Provider请求
	BuildUpstreamRequest(canonical *CanonicalRequest) ([]byte, http.Header, error)

	// 将上游响应转换为内部统一格式
	ParseUpstreamResponse(resp *http.Response) (*CanonicalResponse, error)

	// 将内部统一格式转换为外部协议响应
	BuildResponse(canonical *CanonicalResponse) ([]byte, http.Header, error)

	// 获取协议名称
	Protocol() string
}

// UnifiedProvider 统一Provider接口
type UnifiedProvider interface {
	// 支持的协议列表
	SupportedProtocols() []string

	// 构建请求（通过协议适配器）
	BuildRequest(ctx context.Context, protocol string, rawBody []byte) (*http.Request, error)

	// 处理响应（通过协议适配器）
	ProcessResponse(ctx context.Context, protocol string, resp *http.Response) (*http.Response, error)

	// 获取模型列表
	Models(ctx context.Context) ([]Model, error)
}

// Model 模型信息
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}