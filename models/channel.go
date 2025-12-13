package models

import (
	"time"

	"gorm.io/gorm"
)

// Channel 渠道表 - AxonHub风格的Channel抽象
type Channel struct {
	gorm.Model
	Name                string                 `json:"name" gorm:"uniqueIndex"`                    // 渠道名称
	Description         string                 `json:"description"`                                // 渠道描述
	ProviderID          uint                   `json:"provider_id" gorm:"index"`                   // 关联的Provider ID
	Provider            Provider               `json:"provider" gorm:"foreignKey:ProviderID"`     // Provider关联
	SupportedProtocols  []string               `json:"supported_protocols" gorm:"serializer:json"` // 支持的协议列表
	LoadBalanceStrategy string                 `json:"load_balance_strategy" gorm:"default:error_aware"` // 负载均衡策略
	Weight              int                    `json:"weight" gorm:"default:1"`                    // 权重
	Status              string                 `json:"status" gorm:"default:active"`               // 状态: active, inactive, cooldown
	ParameterOverrides  map[string]interface{} `json:"parameter_overrides" gorm:"serializer:json"` // 参数覆盖

	// 统计信息
	TotalRequests   int64     `json:"total_requests" gorm:"default:0"`   // 总请求数
	SuccessRequests int64     `json:"success_requests" gorm:"default:0"` // 成功请求数
	ErrorRequests   int64     `json:"error_requests" gorm:"default:0"`   // 错误请求数
	AvgResponseTime int64     `json:"avg_response_time" gorm:"default:0"` // 平均响应时间(毫秒)
	LastUsedAt      *time.Time `json:"last_used_at"`                      // 最后使用时间

	// 冷却相关
	CooldownUntil *time.Time `json:"cooldown_until"` // 冷却截止时间
	CooldownStep  int        `json:"cooldown_step"`  // 冷却步数

	// 关联关系
	ModelMappings []ModelMapping `json:"model_mappings" gorm:"foreignKey:ChannelID"` // 模型映射
}

// ModelMapping 模型映射表 - 虚拟模型到实际模型的映射
type ModelMapping struct {
	gorm.Model
	ChannelID           uint                   `json:"channel_id" gorm:"index"`                    // 所属Channel
	Channel             Channel                `json:"channel" gorm:"foreignKey:ChannelID"`       // Channel关联
	VirtualModel        string                 `json:"virtual_model" gorm:"index"`                 // 虚拟模型名称
	ActualModel         string                 `json:"actual_model"`                               // 实际模型名称
	Protocol            string                 `json:"protocol"`                                   // 使用的协议
	ParameterOverrides  map[string]interface{} `json:"parameter_overrides" gorm:"serializer:json"` // 参数覆盖
	Weight              int                    `json:"weight" gorm:"default:1"`                    // 权重
	Status              string                 `json:"status" gorm:"default:active"`               // 状态

	// 统计信息
	RequestCount    int64     `json:"request_count" gorm:"default:0"`    // 请求次数
	SuccessCount    int64     `json:"success_count" gorm:"default:0"`    // 成功次数
	ErrorCount      int64     `json:"error_count" gorm:"default:0"`      // 错误次数
	LastUsedAt      *time.Time `json:"last_used_at"`                      // 最后使用时间
	AvgResponseTime int64     `json:"avg_response_time" gorm:"default:0"` // 平均响应时间
}

// ChannelStats Channel统计信息
type ChannelStats struct {
	ChannelID       uint    `json:"channel_id"`
	ChannelName     string  `json:"channel_name"`
	TotalRequests   int64   `json:"total_requests"`
	SuccessRequests int64   `json:"success_requests"`
	ErrorRequests   int64   `json:"error_requests"`
	ErrorRate       float64 `json:"error_rate"`
	AvgResponseTime int64   `json:"avg_response_time"`
	Status          string  `json:"status"`
	LastUsedAt      *time.Time `json:"last_used_at"`
}

// GetErrorRate 获取错误率
func (c *Channel) GetErrorRate() float64 {
	if c.TotalRequests == 0 {
		return 0
	}
	return float64(c.ErrorRequests) / float64(c.TotalRequests)
}

// GetSuccessRate 获取成功率
func (c *Channel) GetSuccessRate() float64 {
	if c.TotalRequests == 0 {
		return 0
	}
	return float64(c.SuccessRequests) / float64(c.TotalRequests)
}

// IsActive 检查Channel是否可用
func (c *Channel) IsActive() bool {
	if c.Status != "active" {
		return false
	}

	// 检查是否在冷却期
	if c.CooldownUntil != nil && time.Now().Before(*c.CooldownUntil) {
		return false
	}

	return true
}

// UpdateStats 更新统计信息
func (c *Channel) UpdateStats(success bool, responseTime time.Duration) {
	c.TotalRequests++

	if success {
		c.SuccessRequests++
	} else {
		c.ErrorRequests++
	}

	// 更新平均响应时间
	if c.AvgResponseTime == 0 {
		c.AvgResponseTime = responseTime.Milliseconds()
	} else {
		c.AvgResponseTime = (c.AvgResponseTime + responseTime.Milliseconds()) / 2
	}

	now := time.Now()
	c.LastUsedAt = &now
}

// SetCooldown 设置冷却时间
func (c *Channel) SetCooldown(duration time.Duration) {
	c.CooldownStep++
	cooldownUntil := time.Now().Add(duration)
	c.CooldownUntil = &cooldownUntil
	c.Status = "cooldown"
}

// ClearCooldown 清除冷却状态
func (c *Channel) ClearCooldown() {
	c.CooldownUntil = nil
	c.CooldownStep = 0
	c.Status = "active"
}

// GetErrorRate 获取模型映射错误率
func (m *ModelMapping) GetErrorRate() float64 {
	if m.RequestCount == 0 {
		return 0
	}
	return float64(m.ErrorCount) / float64(m.RequestCount)
}

// UpdateStats 更新模型映射统计信息
func (m *ModelMapping) UpdateStats(success bool, responseTime time.Duration) {
	m.RequestCount++

	if success {
		m.SuccessCount++
	} else {
		m.ErrorCount++
	}

	// 更新平均响应时间
	if m.AvgResponseTime == 0 {
		m.AvgResponseTime = responseTime.Milliseconds()
	} else {
		m.AvgResponseTime = (m.AvgResponseTime + responseTime.Milliseconds()) / 2
	}

	now := time.Now()
	m.LastUsedAt = &now
}

// IsActive 检查模型映射是否可用
func (m *ModelMapping) IsActive() bool {
	return m.Status == "active"
}