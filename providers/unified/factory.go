package unified

import (
	"fmt"

	"github.com/atopos31/llmio/consts"
	"github.com/atopos31/llmio/providers"
	"github.com/atopos31/llmio/providers/unified/adapters"
)

// CreateUnifiedProvider 创建统一Provider的工厂函数
func CreateUnifiedProvider(config ProviderConfig) (*Provider, error) {
	// 根据配置创建适配器映射
	adapterMap := make(map[string]ProtocolAdapter)

	// 根据上游类型和支持的协议创建适配器
	for _, protocol := range config.Protocols {
		adapter, err := createAdapter(config.Type, protocol)
		if err != nil {
			return nil, fmt.Errorf("create adapter for protocol %s: %w", protocol, err)
		}
		adapterMap[protocol] = adapter
	}

	return NewProvider(config, adapterMap)
}

// createAdapter 根据上游类型和协议创建适配器
func createAdapter(upstreamType, protocol string) (ProtocolAdapter, error) {
	switch upstreamType {
	case consts.StyleOpenAI:
		// OpenAI上游支持的协议
		switch protocol {
		case "openai":
			return adapters.NewOpenAIAdapter(), nil
		default:
			return nil, fmt.Errorf("openai upstream doesn't support protocol: %s", protocol)
		}

	case consts.StyleAnthropic:
		// Anthropic上游支持的协议
		switch protocol {
		case "anthropic":
			return adapters.NewAnthropicAdapter(), nil
		case "openai":
			// 这是关键：使用OpenAI到Anthropic的转换器
			return adapters.NewOpenAIToAnthropicConverter(), nil
		default:
			return nil, fmt.Errorf("anthropic upstream doesn't support protocol: %s", protocol)
		}

	default:
		return nil, fmt.Errorf("unsupported upstream type: %s", upstreamType)
	}
}

// CreateOpenAICompatibleProvider 创建支持OpenAI协议的统一Provider
// 这个函数让用户可以用一个OpenAI配置支持所有协议
func CreateOpenAICompatibleProvider(baseURL, apiKey string, keys []KeyConfig) (*Provider, error) {
	config := ProviderConfig{
		Type:    consts.StyleOpenAI,
		BaseURL: baseURL,
		APIKey:  apiKey,
		Keys:    convertKeys(keys),
		Protocols: []string{
			"openai",      // 原生OpenAI协议
			// 未来可以添加更多协议支持
		},
	}

	return CreateUnifiedProvider(config)
}

// CreateAnthropicCompatibleProvider 创建支持多协议的Anthropic Provider
// 这是革命性的功能：一个Anthropic配置支持OpenAI协议！
func CreateAnthropicCompatibleProvider(baseURL, apiKey, version string, keys []KeyConfig) (*Provider, error) {
	config := ProviderConfig{
		Type:    consts.StyleAnthropic,
		BaseURL: baseURL,
		APIKey:  apiKey,
		Version: version,
		Keys:    convertKeys(keys),
		Protocols: []string{
			"anthropic",   // 原生Anthropic协议
			"openai",      // 通过转换器支持OpenAI协议！
		},
	}

	return CreateUnifiedProvider(config)
}

// convertKeys 转换Key配置格式
func convertKeys(keys []KeyConfig) []providers.KeyConfig {
	var result []providers.KeyConfig
	for _, key := range keys {
		result = append(result, providers.KeyConfig{
			Term:   key.Term,
			Remark: key.Remark,
			Status: key.Status,
		})
	}
	return result
}

// KeyConfig 统一的Key配置
type KeyConfig struct {
	Term   string `json:"term"`
	Remark string `json:"remark"`
	Status bool   `json:"status"`
}