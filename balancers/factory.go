package balancers

import (
	"fmt"
	"strings"
)

// BalancerType 负载均衡器类型
type BalancerType string

const (
	// 现有策略
	LotteryType          BalancerType = "lottery"
	RotorType            BalancerType = "rotor"
	SmoothWeightedRRType BalancerType = "smooth_weighted_rr"

	// 新增AxonHub策略
	ErrorAwareType       BalancerType = "error_aware"
	TraceAwareType       BalancerType = "trace_aware"
	WeightRoundRobinType BalancerType = "weight_round_robin"
	ConnectionAwareType  BalancerType = "connection_aware"
)

// BalancerFactory 负载均衡器工厂
type BalancerFactory struct{}

// NewBalancerFactory 创建负载均衡器工厂
func NewBalancerFactory() *BalancerFactory {
	return &BalancerFactory{}
}

// CreateBalancer 创建指定类型的负载均衡器
func (f *BalancerFactory) CreateBalancer(balancerType BalancerType, items map[uint]int) (Balancer, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items provided for balancer")
	}

	switch strings.ToLower(string(balancerType)) {
	case string(LotteryType):
		return NewLottery(items), nil
	case string(RotorType):
		return NewRotor(items), nil
	case string(SmoothWeightedRRType):
		return NewSmoothWeightedRR(items), nil
	case string(ErrorAwareType):
		return NewErrorAwareBalancer(items), nil
	case string(TraceAwareType):
		return NewTraceAwareBalancer(items), nil
	case string(WeightRoundRobinType):
		return NewWeightRoundRobinBalancer(items), nil
	case string(ConnectionAwareType):
		return NewConnectionAwareBalancer(items), nil
	default:
		return nil, fmt.Errorf("unsupported balancer type: %s", balancerType)
	}
}

// GetSupportedTypes 获取支持的负载均衡器类型
func (f *BalancerFactory) GetSupportedTypes() []BalancerType {
	return []BalancerType{
		LotteryType,
		RotorType,
		SmoothWeightedRRType,
		ErrorAwareType,
		TraceAwareType,
		WeightRoundRobinType,
		ConnectionAwareType,
	}
}

// GetTypeDescription 获取负载均衡器类型描述
func (f *BalancerFactory) GetTypeDescription(balancerType BalancerType) string {
	descriptions := map[BalancerType]string{
		LotteryType:          "按权重概率抽取，类似抽签",
		RotorType:            "按顺序循环轮转，每次降低权重后移到队尾",
		SmoothWeightedRRType: "平滑加权轮询，避免权重差异过大时的突发流量",
		ErrorAwareType:       "基于错误率选择，优先选择错误率最低的Provider",
		TraceAwareType:       "基于响应时间选择，优先选择响应最快的Provider",
		WeightRoundRobinType: "改进的加权轮询，支持动态权重调整",
		ConnectionAwareType:  "基于连接数选择，优先选择连接数最少的Provider",
	}
	return descriptions[balancerType]
}

// ValidateBalancerType 验证负载均衡器类型
func (f *BalancerFactory) ValidateBalancerType(balancerType string) error {
	supportedTypes := f.GetSupportedTypes()
	for _, supportedType := range supportedTypes {
		if strings.EqualFold(string(supportedType), balancerType) {
			return nil
		}
	}
	return fmt.Errorf("unsupported balancer type: %s", balancerType)
}

// GetDefaultBalancerType 获取默认负载均衡器类型
func (f *BalancerFactory) GetDefaultBalancerType() BalancerType {
	return ErrorAwareType // 默认使用错误感知策略
}

// IsAdvancedBalancer 检查是否为高级负载均衡器（支持统计信息）
func (f *BalancerFactory) IsAdvancedBalancer(balancerType BalancerType) bool {
	advancedTypes := []BalancerType{
		ErrorAwareType,
		TraceAwareType,
		ConnectionAwareType,
	}

	for _, advancedType := range advancedTypes {
		if balancerType == advancedType {
			return true
		}
	}
	return false
}

// BalancerConfig 负载均衡器配置
type BalancerConfig struct {
	Type        BalancerType `json:"type"`
	Description string       `json:"description"`
	Items       map[uint]int `json:"items"`
	Advanced    bool         `json:"advanced"`
}

// CreateBalancerFromConfig 从配置创建负载均衡器
func (f *BalancerFactory) CreateBalancerFromConfig(config *BalancerConfig) (Balancer, error) {
	if config == nil {
		return nil, fmt.Errorf("balancer config is nil")
	}

	if err := f.ValidateBalancerType(string(config.Type)); err != nil {
		return nil, err
	}

	return f.CreateBalancer(config.Type, config.Items)
}

// GetBalancerConfig 获取负载均衡器配置信息
func (f *BalancerFactory) GetBalancerConfig(balancerType BalancerType, items map[uint]int) *BalancerConfig {
	return &BalancerConfig{
		Type:        balancerType,
		Description: f.GetTypeDescription(balancerType),
		Items:       items,
		Advanced:    f.IsAdvancedBalancer(balancerType),
	}
}
