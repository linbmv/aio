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

// OpenAIInboundTransformer OpenAI入站协议转换器
type OpenAIInboundTransformer struct{}

// NewOpenAIInboundTransformer 创建OpenAI入站转换器
func NewOpenAIInboundTransformer() *OpenAIInboundTransformer {
	return &OpenAIInboundTransformer{}
}

// Protocol 获取协议名称
func (t *OpenAIInboundTransformer) Protocol() string {
	return "openai"
}

// CanHandle 检测是否支持该请求
func (t *OpenAIInboundTransformer) CanHandle(request *http.Request) bool {
	// 检查URL路径
	if strings.Contains(request.URL.Path, "/v1/chat/completions") {
		return true
	}

	// 检查Content-Type
	contentType := request.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		return true
	}

	return false
}

// Transform 将OpenAI请求转换为内部统一格式
func (t *OpenAIInboundTransformer) Transform(ctx context.Context, request *http.Request) (*unified.CanonicalRequest, error) {
	// 读取请求体
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// 解析OpenAI请求格式
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(body, &openaiReq); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI request: %w", err)
	}

	// 转换为统一格式
	canonical := &unified.CanonicalRequest{
		Model:       openaiReq.Model,
		MaxTokens:   openaiReq.MaxTokens,
		Temperature: openaiReq.Temperature,
		TopP:        openaiReq.TopP,
		Stop:        openaiReq.Stop,
		Stream:      openaiReq.Stream,
		Metadata:    make(map[string]interface{}),
	}

	// 转换消息
	canonical.Messages = make([]unified.CanonicalMessage, len(openaiReq.Messages))
	for i, msg := range openaiReq.Messages {
		canonical.Messages[i] = unified.CanonicalMessage{
			Role:     msg.Role,
			Name:     msg.Name,
			Metadata: make(map[string]interface{}),
		}

		// 处理消息内容
		if msg.Content != nil {
			switch content := msg.Content.(type) {
			case string:
				// 简单文本内容
				canonical.Messages[i].Content = []unified.CanonicalContent{
					{
						Type: "text",
						Text: content,
					},
				}
			case []interface{}:
				// 复杂内容数组
				canonical.Messages[i].Content = make([]unified.CanonicalContent, len(content))
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

						canonical.Messages[i].Content[j] = canonicalContent
					}
				}
			}
		}
	}

	// 转换工具
	if openaiReq.Tools != nil {
		canonical.Tools = make([]unified.CanonicalTool, len(openaiReq.Tools))
		for i, tool := range openaiReq.Tools {
			if tool.Function != nil {
				canonical.Tools[i] = unified.CanonicalTool{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				}
			}
		}
	}

	// 添加元数据
	canonical.Metadata["original_protocol"] = "openai"
	canonical.Metadata["user_agent"] = request.Header.Get("User-Agent")
	canonical.Metadata["authorization"] = request.Header.Get("Authorization")

	return canonical, nil
}

// OpenAI请求结构体
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
}

type OpenAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content,omitempty"`
	Name    string      `json:"name,omitempty"`
}

type OpenAITool struct {
	Type     string          `json:"type"`
	Function *OpenAIFunction `json:"function,omitempty"`
}

type OpenAIFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}
