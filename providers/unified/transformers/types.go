package transformers

import (
	"context"
	"net/http"
	"time"

	"github.com/linbmv/aio/providers/unified"
)

// InboundTransformer 入站协议转换器 - 将外部协议转换为内部统一格式
type InboundTransformer interface {
	// 将外部协议请求转换为内部统一格式
	Transform(ctx context.Context, request *http.Request) (*unified.CanonicalRequest, error)

	// 获取协议名称
	Protocol() string

	// 检测是否支持该请求
	CanHandle(request *http.Request) bool
}

// OutboundTransformer 出站协议转换器 - 将内部统一格式转换为上游Provider协议
type OutboundTransformer interface {
	// 将内部统一格式转换为上游Provider请求
	Transform(ctx context.Context, canonical *unified.CanonicalRequest, endpoint string) (*http.Request, error)

	// 将上游响应转换为内部统一格式
	ParseResponse(ctx context.Context, resp *http.Response) (*unified.CanonicalResponse, error)

	// 获取协议名称
	Protocol() string

	// 检测是否支持该Provider
	CanHandle(providerType string) bool
}

// ResponseTransformer 响应转换器 - 将内部统一格式转换为外部协议响应
type ResponseTransformer interface {
	// 将内部统一格式转换为外部协议响应
	Transform(ctx context.Context, canonical *unified.CanonicalResponse, originalProtocol string) (*http.Response, error)

	// 获取协议名称
	Protocol() string
}

// TransformerRegistry 转换器注册表
type TransformerRegistry struct {
	inboundTransformers  map[string]InboundTransformer
	outboundTransformers map[string]OutboundTransformer
	responseTransformers map[string]ResponseTransformer
}

// NewTransformerRegistry 创建转换器注册表
func NewTransformerRegistry() *TransformerRegistry {
	return &TransformerRegistry{
		inboundTransformers:  make(map[string]InboundTransformer),
		outboundTransformers: make(map[string]OutboundTransformer),
		responseTransformers: make(map[string]ResponseTransformer),
	}
}

// RegisterInbound 注册入站转换器
func (r *TransformerRegistry) RegisterInbound(transformer InboundTransformer) {
	r.inboundTransformers[transformer.Protocol()] = transformer
}

// RegisterOutbound 注册出站转换器
func (r *TransformerRegistry) RegisterOutbound(transformer OutboundTransformer) {
	r.outboundTransformers[transformer.Protocol()] = transformer
}

// RegisterResponse 注册响应转换器
func (r *TransformerRegistry) RegisterResponse(transformer ResponseTransformer) {
	r.responseTransformers[transformer.Protocol()] = transformer
}

// GetInbound 获取入站转换器
func (r *TransformerRegistry) GetInbound(protocol string) InboundTransformer {
	return r.inboundTransformers[protocol]
}

// GetOutbound 获取出站转换器
func (r *TransformerRegistry) GetOutbound(protocol string) OutboundTransformer {
	return r.outboundTransformers[protocol]
}

// GetResponse 获取响应转换器
func (r *TransformerRegistry) GetResponse(protocol string) ResponseTransformer {
	return r.responseTransformers[protocol]
}

// DetectInboundProtocol 自动检测入站协议
func (r *TransformerRegistry) DetectInboundProtocol(request *http.Request) InboundTransformer {
	for _, transformer := range r.inboundTransformers {
		if transformer.CanHandle(request) {
			return transformer
		}
	}
	return nil
}

// DetectOutboundProtocol 自动检测出站协议
func (r *TransformerRegistry) DetectOutboundProtocol(providerType string) OutboundTransformer {
	for _, transformer := range r.outboundTransformers {
		if transformer.CanHandle(providerType) {
			return transformer
		}
	}
	return nil
}

// TransformationContext 转换上下文
type TransformationContext struct {
	RequestID     string
	UserAgent     string
	ClientIP      string
	AuthKeyID     uint
	StartTime     time.Time
	Metadata      map[string]interface{}
}

// NewTransformationContext 创建转换上下文
func NewTransformationContext(requestID string) *TransformationContext {
	return &TransformationContext{
		RequestID: requestID,
		StartTime: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}