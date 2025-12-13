package transformers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/linbmv/aio/providers/unified"
)

// TransformerManager 转换器管理器 - 协调整个转换流程
type TransformerManager struct {
	registry *TransformerRegistry
}

// NewTransformerManager 创建转换器管理器
func NewTransformerManager() *TransformerManager {
	registry := NewTransformerRegistry()

	// 注册所有转换器
	registry.RegisterInbound(NewOpenAIInboundTransformer())
	registry.RegisterInbound(NewAnthropicInboundTransformer())

	registry.RegisterOutbound(NewOpenAIOutboundTransformer())
	registry.RegisterOutbound(NewAnthropicOutboundTransformer())

	registry.RegisterResponse(NewOpenAIResponseTransformer())
	registry.RegisterResponse(NewAnthropicResponseTransformer())

	return &TransformerManager{
		registry: registry,
	}
}

// ProcessRequest 处理完整的请求转换流程
func (m *TransformerManager) ProcessRequest(ctx context.Context, clientReq *http.Request, providerType, providerEndpoint string) (*unified.CanonicalRequest, *http.Request, string, error) {
	// 1. 自动检测入站协议
	inboundTransformer := m.registry.DetectInboundProtocol(clientReq)
	if inboundTransformer == nil {
		return nil, nil, "", fmt.Errorf("unsupported inbound protocol")
	}

	clientProtocol := inboundTransformer.Protocol()

	// 2. 将客户端请求转换为统一格式
	canonical, err := inboundTransformer.Transform(ctx, clientReq)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to transform inbound request: %w", err)
	}

	// 3. 检测出站协议
	outboundTransformer := m.registry.DetectOutboundProtocol(providerType)
	if outboundTransformer == nil {
		return nil, nil, "", fmt.Errorf("unsupported provider type: %s", providerType)
	}

	// 4. 将统一格式转换为Provider请求
	providerReq, err := outboundTransformer.Transform(ctx, canonical, providerEndpoint)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to transform outbound request: %w", err)
	}

	return canonical, providerReq, clientProtocol, nil
}

// ProcessResponse 处理完整的响应转换流程
func (m *TransformerManager) ProcessResponse(ctx context.Context, providerResp *http.Response, providerType, clientProtocol string) (*http.Response, error) {
	// 1. 获取出站转换器解析Provider响应
	outboundTransformer := m.registry.DetectOutboundProtocol(providerType)
	if outboundTransformer == nil {
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}

	// 2. 将Provider响应转换为统一格式
	canonical, err := outboundTransformer.ParseResponse(ctx, providerResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse provider response: %w", err)
	}

	// 3. 获取响应转换器
	responseTransformer := m.registry.GetResponse(clientProtocol)
	if responseTransformer == nil {
		return nil, fmt.Errorf("unsupported client protocol: %s", clientProtocol)
	}

	// 4. 将统一格式转换为客户端协议响应
	clientResp, err := responseTransformer.Transform(ctx, canonical, outboundTransformer.Protocol())
	if err != nil {
		return nil, fmt.Errorf("failed to transform response: %w", err)
	}

	return clientResp, nil
}

// GetSupportedProtocols 获取支持的协议列表
func (m *TransformerManager) GetSupportedProtocols() map[string][]string {
	return map[string][]string{
		"inbound":  {"openai", "anthropic"},
		"outbound": {"openai", "anthropic"},
		"response": {"openai", "anthropic"},
	}
}

// ValidateTransformation 验证转换配置
func (m *TransformerManager) ValidateTransformation(clientProtocol, providerType string) error {
	// 检查入站协议支持
	inboundTransformer := m.registry.GetInbound(clientProtocol)
	if inboundTransformer == nil {
		return fmt.Errorf("unsupported client protocol: %s", clientProtocol)
	}

	// 检查出站协议支持
	outboundTransformer := m.registry.DetectOutboundProtocol(providerType)
	if outboundTransformer == nil {
		return fmt.Errorf("unsupported provider type: %s", providerType)
	}

	// 检查响应协议支持
	responseTransformer := m.registry.GetResponse(clientProtocol)
	if responseTransformer == nil {
		return fmt.Errorf("unsupported response protocol: %s", clientProtocol)
	}

	return nil
}

// GetTransformationPath 获取转换路径信息
func (m *TransformerManager) GetTransformationPath(clientProtocol, providerType string) (string, error) {
	if err := m.ValidateTransformation(clientProtocol, providerType); err != nil {
		return "", err
	}

	outboundTransformer := m.registry.DetectOutboundProtocol(providerType)
	providerProtocol := outboundTransformer.Protocol()

	return fmt.Sprintf("%s -> canonical -> %s -> canonical -> %s",
		clientProtocol, providerProtocol, clientProtocol), nil
}