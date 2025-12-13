package unified

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/atopos31/llmio/providers"
)

// Provider 统一Provider实现
type Provider struct {
	// 上游Provider（实际的API提供商，如OpenAI）
	upstream providers.Provider

	// 协议适配器映射
	adapters map[string]ProtocolAdapter

	// 配置信息
	config ProviderConfig
}

// ProviderConfig 统一Provider配置
type ProviderConfig struct {
	Type        string                 `json:"type"`         // 上游类型：openai, anthropic
	BaseURL     string                 `json:"base_url"`
	APIKey      string                 `json:"api_key"`
	Keys        []providers.KeyConfig  `json:"keys"`
	Version     string                 `json:"version,omitempty"`
	Protocols   []string               `json:"protocols"`    // 支持的协议列表
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// NewProvider 创建统一Provider
func NewProvider(config ProviderConfig, adapters map[string]ProtocolAdapter) (*Provider, error) {
	// 创建上游Provider
	upstreamConfig, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal upstream config: %w", err)
	}

	upstream, err := providers.New(config.Type, string(upstreamConfig))
	if err != nil {
		return nil, fmt.Errorf("create upstream provider: %w", err)
	}

	return &Provider{
		upstream: upstream,
		adapters: adapters,
		config:   config,
	}, nil
}

func (p *Provider) SupportedProtocols() []string {
	return p.config.Protocols
}

func (p *Provider) BuildRequest(ctx context.Context, protocol string, rawBody []byte) (*http.Request, error) {
	// 获取协议适配器
	adapter, exists := p.adapters[protocol]
	if !exists {
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	// 1. 解析外部协议请求为canonical格式
	canonical, err := adapter.ParseRequest(rawBody)
	if err != nil {
		return nil, fmt.Errorf("parse request: %w", err)
	}

	// 2. 将canonical格式转换为上游Provider请求
	upstreamBody, upstreamHeaders, err := adapter.BuildUpstreamRequest(canonical)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	// 3. 使用上游Provider构建实际请求
	req, err := p.upstream.BuildReq(ctx, upstreamHeaders, canonical.Model, upstreamBody)
	if err != nil {
		return nil, fmt.Errorf("build upstream req: %w", err)
	}

	return req, nil
}

func (p *Provider) ProcessResponse(ctx context.Context, protocol string, resp *http.Response) (*http.Response, error) {
	// 获取协议适配器
	adapter, exists := p.adapters[protocol]
	if !exists {
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}

	// 1. 解析上游响应为canonical格式
	canonical, err := adapter.ParseUpstreamResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("parse upstream response: %w", err)
	}

	// 2. 将canonical格式转换为外部协议响应
	responseBody, responseHeaders, err := adapter.BuildResponse(canonical)
	if err != nil {
		return nil, fmt.Errorf("build response: %w", err)
	}

	// 3. 构建新的响应
	newResp := &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        responseHeaders,
		Body:          io.NopCloser(bytes.NewReader(responseBody)),
		ContentLength: int64(len(responseBody)),
		Request:       resp.Request,
	}

	return newResp, nil
}

func (p *Provider) Models(ctx context.Context) ([]Model, error) {
	upstreamModels, err := p.upstream.Models(ctx)
	if err != nil {
		return nil, err
	}

	// 转换为统一格式
	models := make([]Model, len(upstreamModels))
	for i, model := range upstreamModels {
		models[i] = Model{
			ID:      model.ID,
			Object:  model.Object,
			Created: model.Created,
			OwnedBy: model.OwnedBy,
		}
	}

	return models, nil
}