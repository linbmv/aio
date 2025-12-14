package models

import (
	"time"

	"gorm.io/gorm"
)

// ProviderKey Key 池管理
type ProviderKey struct {
	gorm.Model
	ProviderID    uint   `gorm:"not null;index:idx_provider_key,priority:1"`
	Key           string `gorm:"type:text;not null;index:idx_provider_key,priority:2"` // API Key
	Remark        string
	Status        bool       `gorm:"not null;default:true;index"` // 是否启用
	CooldownUntil *time.Time `gorm:"index"`                       // Key 级冷却截止时间
	CooldownStep  int
	SuccessCount  int64      `gorm:"not null;default:0"` // 成功次数
	FailCount     int64      `gorm:"not null;default:0"` // 失败次数
	LastUsedAt    *time.Time `gorm:"index"`              // 最后使用时间
}
