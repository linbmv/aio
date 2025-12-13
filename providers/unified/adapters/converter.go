package adapters

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atopos31/llmio/providers/unified"
)

// OpenAIToAnthropicConverter OpenAI协议到Anthropic上游的转换器
type OpenAIToAnthropicConverter struct {
	openaiAdapter    *OpenAIAdapter
	anthropicAdapter *AnthropicAdapter
}

func NewOpenAIToAnthropicConverter() *OpenAIToAnthropicConverter {
	return &OpenAIToAnthropicConverter{
		openaiAdapter:    NewOpenAIAdapter(),
		anthropicAdapter: NewAnthropicAdapter(),
	}
}

func (c *OpenAIToAnthropicConverter) Protocol() string {
	return "openai"
}

func (c *OpenAIToAnthropicConverter) ParseRequest(rawBody []byte) (*unified.CanonicalRequest, error) {
	// 使用OpenAI适配器解析请求
	return c.openaiAdapter.ParseRequest(rawBody)
}

func (c *OpenAIToAnthropicConverter) BuildUpstreamRequest(canonical *unified.CanonicalRequest) ([]byte, http.Header, error) {
	// 将canonical格式转换为Anthropic格式
	anthropicReq := AnthropicRequest{
		Model:       canonical.Model,
		MaxTokens:   *canonical.MaxTokens,
		Temperature: canonical.Temperature,
		TopP:        canonical.TopP,
		TopK:        canonical.TopK,
		Stream:      canonical.Stream,
		Metadata:    canonical.Metadata,
	}

	// 转换消息格式
	for _, msg := range canonical.Messages {
		anthropicMsg := AnthropicMessage{
			Role: msg.Role,
		}

		for _, content := range msg.Content {
			anthropicContent := AnthropicContent{
				Type: content.Type,
				Text: content.Text,
				Data: content.Data,
			}
			anthropicMsg.Content = append(anthropicMsg.Content, anthropicContent)
		}

		anthropicReq.Messages = append(anthropicReq.Messages, anthropicMsg)
	}

	// 转换工具格式
	for _, tool := range canonical.Tools {
		anthropicTool := AnthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.Parameters,
		}
		anthropicReq.Tools = append(anthropicReq.Tools, anthropicTool)
	}

	// 转换stop序列
	if len(canonical.Stop) > 0 {
		anthropicReq.Stop = canonical.Stop
	}

	// 构建请求体
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	// 构建Anthropic特有的请求头
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("anthropic-version", "2023-06-01")

	return body, headers, nil
}

func (c *OpenAIToAnthropicConverter) ParseUpstreamResponse(resp *http.Response) (*unified.CanonicalResponse, error) {
	// 解析Anthropic响应为canonical格式
	// 这里需要实现Anthropic响应格式的解析
	// 暂时返回基础结构
	return &unified.CanonicalResponse{
		ID:    "chatcmpl-unified",
		Model: "claude-3-opus-20240229",
		Choices: []unified.CanonicalChoice{
			{
				Index: 0,
				Message: unified.CanonicalMessage{
					Role: "assistant",
					Content: []unified.CanonicalContent{
						{Type: "text", Text: "Response from Anthropic"},
					},
				},
				FinishReason: "stop",
			},
		},
		Usage: unified.CanonicalUsage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}, nil
}

func (c *OpenAIToAnthropicConverter) BuildResponse(canonical *unified.CanonicalResponse) ([]byte, http.Header, error) {
	// 将canonical格式转换为OpenAI响应格式
	openaiResp := OpenAIResponse{
		ID:      canonical.ID,
		Object:  "chat.completion",
		Created: canonical.Created.Unix(),
		Model:   canonical.Model,
		Usage: OpenAIUsage{
			PromptTokens:     canonical.Usage.PromptTokens,
			CompletionTokens: canonical.Usage.CompletionTokens,
			TotalTokens:      canonical.Usage.TotalTokens,
		},
	}

	// 转换选择
	for _, choice := range canonical.Choices {
		openaiChoice := OpenAIChoice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
			Message: OpenAIMessage{
				Role: choice.Message.Role,
			},
		}

		// 转换消息内容
		if len(choice.Message.Content) == 1 && choice.Message.Content[0].Type == "text" {
			// 简单文本消息
			openaiChoice.Message.Content = choice.Message.Content[0].Text
		} else {
			// 复杂内容
			var contents []OpenAIContent
			for _, content := range choice.Message.Content {
				contents = append(contents, OpenAIContent{
					Type: content.Type,
					Text: content.Text,
				})
			}
			openaiChoice.Message.Content = contents
		}

		openaiResp.Choices = append(openaiResp.Choices, openaiChoice)
	}

	body, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal openai response: %w", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	return body, headers, nil
}

// OpenAI响应格式定义
type OpenAIResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []OpenAIChoice  `json:"choices"`
	Usage   OpenAIUsage     `json:"usage"`
}

type OpenAIChoice struct {
	Index        int         `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}