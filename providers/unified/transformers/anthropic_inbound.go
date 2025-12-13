package transformers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/linbmv/aio/providers/unified"
)

// AnthropicInboundTransformer Anthropic入站协议转换器
type AnthropicInboundTransformer struct{}

// NewAnthropicInboundTransformer 创建Anthropic入站转换器
func NewAnthropicInboundTransformer() *AnthropicInboundTransformer {
	return &AnthropicInboundTransformer{}
}

// Protocol 获取协议名称
func (t *AnthropicInboundTransformer) Protocol() string {
	return "anthropic"
}

// CanHandle 检测是否支持该请求
func (t *AnthropicInboundTransformer) CanHandle(request *http.Request) bool {
	// 检查URL路径
	if strings.Contains(request.URL.Path, "/v1/messages") {
		return true
	}

	// 检查Anthropic特有的头部
	if request.Header.Get("anthropic-version") != "" {
		return true
	}

	// 检查x-api-key头部（Anthropic特有）
	if request.Header.Get("x-api-key") != "" {
		return true
	}

	return false
}

// Transform 将Anthropic请求转换为内部统一格式
func (t *AnthropicInboundTransformer) Transform(ctx context.Context, request *http.Request) (*unified.CanonicalRequest, error) {
	// 读取请求体
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// 解析Anthropic请求格式
	var anthropicReq AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic request: %w", err)
	}

	// 转换为统一格式
	canonical := &unified.CanonicalRequest{
		Model:       anthropicReq.Model,
		MaxTokens:   &anthropicReq.MaxTokens,
		Temperature: anthropicReq.Temperature,
		TopP:        anthropicReq.TopP,
		TopK:        anthropicReq.TopK,
		Stop:        anthropicReq.StopSequences,
		Stream:      anthropicReq.Stream,
		Metadata:    make(map[string]interface{}),
	}

	// 转换消息 - Anthropic有system和messages分离
	canonical.Messages = make([]unified.CanonicalMessage, 0)

	// 添加system消息（如果存在）
	if anthropicReq.System != "" {
		canonical.Messages = append(canonical.Messages, unified.CanonicalMessage{
			Role: "system",
			Content: []unified.CanonicalContent{
				{
					Type: "text",
					Text: anthropicReq.System,
				},
			},
			Metadata: make(map[string]interface{}),
		})
	}

	// 转换用户和助手消息
	for _, msg := range anthropicReq.Messages {
		canonicalMsg := unified.CanonicalMessage{
			Role:     msg.Role,
			Metadata: make(map[string]interface{}),
		}

		// 转换内容
		canonicalMsg.Content = make([]unified.CanonicalContent, len(msg.Content))
		for i, content := range msg.Content {
			canonicalContent := unified.CanonicalContent{
				Type: content.Type,
				Text: content.Text,
				Data: make(map[string]interface{}),
			}

			// 处理特殊内容类型
			switch content.Type {
			case "image":
				if content.Source != nil {
					canonicalContent.Data["source"] = content.Source
				}
			case "tool_use":
				canonicalContent.Data["id"] = content.ID
				canonicalContent.Data["name"] = content.Name
				canonicalContent.Data["input"] = content.Input
			case "tool_result":
				canonicalContent.Data["tool_use_id"] = content.ToolUseID
				canonicalContent.Data["is_error"] = content.IsError
			}

			canonicalMsg.Content[i] = canonicalContent
		}

		canonical.Messages = append(canonical.Messages, canonicalMsg)
	}

	// 转换工具
	if anthropicReq.Tools != nil {
		canonical.Tools = make([]unified.CanonicalTool, len(anthropicReq.Tools))
		for i, tool := range anthropicReq.Tools {
			canonical.Tools[i] = unified.CanonicalTool{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			}
		}
	}

	// 添加元数据
	canonical.Metadata["original_protocol"] = "anthropic"
	canonical.Metadata["anthropic_version"] = request.Header.Get("anthropic-version")
	canonical.Metadata["x_api_key"] = request.Header.Get("x-api-key")

	return canonical, nil
}

// Anthropic请求结构体
type AnthropicRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	Messages      []AnthropicMessage `json:"messages"`
	System        string             `json:"system,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	TopK          *int               `json:"top_k,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
	Role    string             `json:"role"`
	Content []AnthropicContent `json:"content"`
}

type AnthropicContent struct {
	Type      string                 `json:"type"`
	Text      string                 `json:"text,omitempty"`
	Source    map[string]interface{} `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	IsError   bool                   `json:"is_error,omitempty"`
}

type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}