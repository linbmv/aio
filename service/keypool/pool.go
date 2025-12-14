package keypool

import (
	"context"
	"fmt"
	"time"

	"github.com/atopos31/llmio/models"
	"github.com/atopos31/llmio/service/cooldown"
	"gorm.io/gorm"
)

type Pool struct {
	db *gorm.DB
}

func NewPool(db *gorm.DB) *Pool {
	return &Pool{db: db}
}

// Pick 选择可用的 Key
func (p *Pool) Pick(ctx context.Context, providerID uint) (key string, keyID uint, err error) {
	now := time.Now()
	var keys []models.ProviderKey

	// 查询启用且未冷却的 Key
	keys, err = gorm.G[models.ProviderKey](p.db).
		Where("provider_id = ? AND status = ? AND (cooldown_until IS NULL OR cooldown_until < ?)",
									providerID, true, now).
		Order("last_used_at IS NOT NULL, last_used_at ASC"). // 优先使用最久未用的
		Limit(1).
		Find(ctx)
	if err != nil {
		return "", 0, err
	}

	if len(keys) == 0 {
		return "", 0, fmt.Errorf("no available key for provider %d", providerID)
	}

	// 选择第一个可用 Key
	selected := keys[0]

	// 更新最后使用时间
	nowTime := time.Now()
	selected.LastUsedAt = &nowTime
	if _, err := gorm.G[models.ProviderKey](p.db).
		Where("id = ?", selected.ID).
		Update(ctx, "last_used_at", nowTime); err != nil {
		// 非致命错误，继续
	}

	return selected.Key, selected.ID, nil
}

// OnSuccess Key 使用成功
func (p *Pool) OnSuccess(ctx context.Context, keyID uint) error {
	// 清除冷却状态，增加成功计数，重置失败计数
	updates := models.ProviderKey{
		CooldownUntil: nil,
		CooldownStep:  0,
		FailCount:     0,
	}
	_, err := gorm.G[models.ProviderKey](p.db).
		Where("id = ?", keyID).
		Updates(ctx, updates)
	if err != nil {
		return err
	}
	// 单独更新成功计数（使用表达式）
	_, err = gorm.G[models.ProviderKey](p.db).
		Where("id = ?", keyID).
		Update(ctx, "success_count", gorm.Expr("success_count + 1"))
	return err
}

// OnError Key 使用失败
func (p *Pool) OnError(ctx context.Context, keyID uint, category cooldown.Category) error {
	if category != cooldown.CategoryKey {
		return nil // 非 Key 级错误不处理
	}

	key, err := gorm.G[models.ProviderKey](p.db).
		Where("id = ?", keyID).
		First(ctx)
	if err != nil {
		return err
	}

	// 指数退避冷却
	key.CooldownStep++
	until := p.nextCooldownTime(key.CooldownStep)
	key.CooldownUntil = &until
	key.FailCount++

	// 更新字段
	updates := map[string]interface{}{
		"cooldown_step":  key.CooldownStep,
		"cooldown_until": key.CooldownUntil,
		"fail_count":     key.FailCount,
	}

	// 连续失败 3 次禁用 Key
	if key.FailCount >= 3 {
		updates["status"] = false
	}

	err = p.db.WithContext(ctx).Model(&models.ProviderKey{}).
		Where("id = ?", keyID).
		Updates(updates).Error
	return err
}

func (p *Pool) nextCooldownTime(step int) time.Time {
	if step <= 0 {
		return time.Now().Add(time.Second)
	}
	if step > 20 {
		step = 20
	}
	delay := time.Second << (step - 1)
	maxBackoff := 30 * time.Minute
	if delay > maxBackoff {
		delay = maxBackoff
	}
	return time.Now().Add(delay)
}
