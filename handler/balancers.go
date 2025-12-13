package handler

import (
	"net/http"

	"github.com/atopos31/llmio/balancers"
	"github.com/atopos31/llmio/common"
	"github.com/gin-gonic/gin"
)

// BalancerTypeInfo 负载均衡器类型信息
type BalancerTypeInfo struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Advanced    bool   `json:"advanced"`
}

// GetBalancerTypes 获取支持的负载均衡器类型
func GetBalancerTypes(c *gin.Context) {
	factory := balancers.NewBalancerFactory()
	supportedTypes := factory.GetSupportedTypes()

	var typeInfos []BalancerTypeInfo
	for _, balancerType := range supportedTypes {
		typeInfo := BalancerTypeInfo{
			Type:        string(balancerType),
			Name:        getBalancerTypeName(balancerType),
			Description: factory.GetTypeDescription(balancerType),
			Advanced:    factory.IsAdvancedBalancer(balancerType),
		}
		typeInfos = append(typeInfos, typeInfo)
	}

	common.SuccessResponse(c, typeInfos)
}

// getBalancerTypeName 获取负载均衡器类型的中文名称
func getBalancerTypeName(balancerType balancers.BalancerType) string {
	names := map[balancers.BalancerType]string{
		balancers.LotteryType:           "权重抽签",
		balancers.RotorType:            "循环轮转",
		balancers.SmoothWeightedRRType: "平滑加权轮询",
		balancers.ErrorAwareType:       "错误感知",
		balancers.TraceAwareType:       "响应时间感知",
		balancers.WeightRoundRobinType: "加权轮询",
		balancers.ConnectionAwareType:  "连接数感知",
	}
	return names[balancerType]
}

// GetSupportedProtocols 获取支持的协议列表
func GetSupportedProtocols(c *gin.Context) {
	protocols := []map[string]interface{}{
		{
			"type":        "openai",
			"name":        "OpenAI",
			"description": "OpenAI兼容协议，支持ChatGPT、GPT-4等模型",
		},
		{
			"type":        "anthropic",
			"name":        "Anthropic",
			"description": "Anthropic Claude协议，支持Claude系列模型",
		},
	}

	common.SuccessResponse(c, protocols)
}