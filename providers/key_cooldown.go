package providers

import (
	"fmt"
	"sync"
	"time"
)

// keyCooldownManager key冷却管理，标记失败Key避免短时间重复使用
type keyCooldownManager struct {
	mu       sync.RWMutex
	expireAt map[string]time.Time
	duration time.Duration
}

func newKeyCooldownManager(defaultDuration time.Duration) *keyCooldownManager {
	return &keyCooldownManager{
		expireAt: make(map[string]time.Time),
		duration: defaultDuration,
	}
}

func (m *keyCooldownManager) mark(id string, d time.Duration) {
	if id == "" {
		return
	}
	if d <= 0 {
		d = m.duration
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireAt[id] = time.Now().Add(d)
}

func (m *keyCooldownManager) isCooling(id string, now time.Time) bool {
	if id == "" {
		return false
	}
	m.mu.RLock()
	expire, ok := m.expireAt[id]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	if expire.After(now) {
		return true
	}
	// 过期清理
	m.mu.Lock()
	delete(m.expireAt, id)
	m.mu.Unlock()
	return false
}

var globalKeyCooldown = newKeyCooldownManager(60 * time.Second)

func init() {
	// 启动定期清理过期条目的 goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			globalKeyCooldown.cleanup()
		}
	}()
}

func (m *keyCooldownManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, expireAt := range m.expireAt {
		if expireAt.Before(now) {
			delete(m.expireAt, id)
		}
	}
}

func makeKeyCooldownID(providerID uint, key string) string {
	return fmt.Sprintf("%d:%s", providerID, key)
}

// MarkKeyFailure 将Key标记为冷却
func MarkKeyFailure(providerID uint, key string) {
	if key == "" {
		return
	}
	globalKeyCooldown.mark(makeKeyCooldownID(providerID, key), 0)
}

// IsKeyCoolingDown 判断Key是否仍在冷却
func IsKeyCoolingDown(providerID uint, key string) bool {
	if key == "" {
		return false
	}
	return globalKeyCooldown.isCooling(makeKeyCooldownID(providerID, key), time.Now())
}
