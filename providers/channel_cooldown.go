package providers

import (
	"fmt"
	"sync"
	"time"
)

type channelCooldownManager struct {
	mu           sync.RWMutex
	expireAt     map[string]time.Time
	failureCount map[string]int
	base         time.Duration
}

func newChannelCooldownManager() *channelCooldownManager {
	return &channelCooldownManager{
		expireAt:     make(map[string]time.Time),
		failureCount: make(map[string]int),
		base:         30 * time.Second,
	}
}

func (m *channelCooldownManager) mark(id string, d time.Duration) {
	if id == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	count := m.failureCount[id] + 1
	m.failureCount[id] = count
	if d <= 0 {
		d = calculateBackoff(m.base, count)
	}
	m.expireAt[id] = time.Now().Add(d)
}

func (m *channelCooldownManager) isCooling(id string, now time.Time) bool {
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
	m.mu.Lock()
	delete(m.expireAt, id)
	delete(m.failureCount, id)
	m.mu.Unlock()
	return false
}

func (m *channelCooldownManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, expireAt := range m.expireAt {
		if expireAt.Before(now) {
			delete(m.expireAt, id)
			delete(m.failureCount, id)
		}
	}
}

var globalChannelCooldown = newChannelCooldownManager()

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			globalChannelCooldown.cleanup()
		}
	}()
}

func makeChannelCooldownID(modelID, providerID uint) string {
	return fmt.Sprintf("%d:%d", modelID, providerID)
}

// MarkChannelFailure 标记渠道冷却
func MarkChannelFailure(modelID, providerID uint, duration time.Duration) {
	globalChannelCooldown.mark(makeChannelCooldownID(modelID, providerID), duration)
}

// IsChannelCoolingDown 判断渠道是否冷却中
func IsChannelCoolingDown(modelID, providerID uint) bool {
	return globalChannelCooldown.isCooling(makeChannelCooldownID(modelID, providerID), time.Now())
}
