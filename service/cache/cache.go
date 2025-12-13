package cache

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Scope 定义缓存作用域，确保多租户隔离
type Scope struct {
	AuthKeyID uint   `json:"auth_key_id"`
	Style     string `json:"style"`     // API风格：OpenAI/Anthropic/OpenAIRes
	Model     string `json:"model"`
	Mode      string `json:"mode"`
	Stream    bool   `json:"stream"`
}

// Key 表示缓存键，由作用域和请求体哈希组成
type Key struct {
	Scope    Scope  `json:"scope"`
	BodyHash string `json:"body_hash"`
}

// ReadPolicy 定义缓存读取策略
type ReadPolicy int

const (
	ReadPolicyClone         ReadPolicy = iota // 深拷贝（默认，向后兼容）
	ReadPolicyShareReadOnly                   // 只读共享模式
)

// Options 缓存配置选项
type Options struct {
	MaxEntries     int        // 最大缓存条目数
	ReadPolicy     ReadPolicy // 读取策略
	ShareThreshold int        // 共享阈值（字节），仅大于此值的响应使用共享模式
}

// Value 表示缓存的响应数据
type Value struct {
	StatusCode    int         `json:"status_code"`
	Header        http.Header `json:"header"`
	Body          []byte      `json:"body"`
	CreatedAt     time.Time   `json:"created_at"`
	ExpiresAt     time.Time   `json:"expires_at"`

	// 审计相关字段
	SourceLogID   uint        `json:"source_log_id"`   // 最初生成缓存的日志ID
	Usage         interface{} `json:"usage"`           // 原始Usage信息
	ProviderName  string      `json:"provider_name"`   // Provider名称
	ProviderModel string      `json:"provider_model"`  // Provider模型

	// 性能优化字段
	Shared bool `json:"shared"` // 标记是否为共享引用（只读）
}

// Cache 定义缓存接口，支持多种后端实现
type Cache interface {
	// Get 获取缓存数据，返回值、是否命中、错误
	Get(ctx context.Context, key Key) (*Value, bool, error)

	// Set 设置缓存数据，指定TTL
	Set(ctx context.Context, key Key, value *Value, ttl time.Duration) error

	// DeleteByAuthKey 按AuthKeyID清空对应租户的所有缓存
	DeleteByAuthKey(ctx context.Context, authKeyID uint) error

	// DeleteByStyle 按API风格清空对应的所有缓存
	DeleteByStyle(ctx context.Context, style string) error

	// Stats 获取缓存统计信息
	Stats() CacheStats
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Entries   int `json:"entries"`
	HitCount  int `json:"hit_count"`
	MissCount int `json:"miss_count"`
}

// entry 内存缓存的单条记录
type entry struct {
	key   Key
	value *Value
}

// MemoryCache 线程安全的内存缓存实现
type MemoryCache struct {
	mu             sync.RWMutex
	data           map[string]entry
	maxEntries     int
	readPolicy     ReadPolicy
	shareThreshold int
	hitCount       int
	missCount      int
}

const (
	// DefaultMaxEntries 默认最大缓存条目数
	DefaultMaxEntries = 1024
	// DefaultTTL 默认缓存过期时间
	DefaultTTL = time.Minute * 5
)

// NewMemoryCache 创建内存缓存实例（向后兼容）
func NewMemoryCache(maxEntries int) *MemoryCache {
	return NewMemoryCacheWithOptions(Options{
		MaxEntries:     maxEntries,
		ReadPolicy:     ReadPolicyClone, // 默认深拷贝，保持兼容性
		ShareThreshold: 0,
	})
}

// NewMemoryCacheWithOptions 使用配置选项创建内存缓存实例
func NewMemoryCacheWithOptions(opts Options) *MemoryCache {
	if opts.MaxEntries <= 0 {
		opts.MaxEntries = DefaultMaxEntries
	}
	return &MemoryCache{
		data:           make(map[string]entry, opts.MaxEntries),
		maxEntries:     opts.MaxEntries,
		readPolicy:     opts.ReadPolicy,
		shareThreshold: opts.ShareThreshold,
	}
}

// Get 获取缓存数据，自动处理过期清理
func (c *MemoryCache) Get(ctx context.Context, key Key) (*Value, bool, error) {
	mapKey := c.makeMapKey(key)
	now := time.Now()

	c.mu.RLock()
	e, exists := c.data[mapKey]
	c.mu.RUnlock()

	if !exists {
		c.mu.Lock()
		c.missCount++
		c.mu.Unlock()
		return nil, false, nil
	}

	// 检查过期
	if !e.value.ExpiresAt.IsZero() && now.After(e.value.ExpiresAt) {
		c.mu.Lock()
		// 双重检查，避免并发问题
		if e, exists = c.data[mapKey]; exists && !e.value.ExpiresAt.IsZero() && now.After(e.value.ExpiresAt) {
			delete(c.data, mapKey)
		}
		c.missCount++
		c.mu.Unlock()
		return nil, false, nil
	}

	c.mu.Lock()
	c.hitCount++
	c.mu.Unlock()

	// 根据策略决定是否共享引用
	shareAllowed := c.readPolicy == ReadPolicyShareReadOnly
	bigEnough := c.shareThreshold <= 0 || len(e.value.Body) >= c.shareThreshold

	if shareAllowed && bigEnough {
		// 只读共享模式：直接返回引用，标记为共享
		shared := *e.value
		shared.Shared = true
		return &shared, true, nil
	}

	// 深拷贝模式：返回副本，避免调用方修改内部状态
	cloned := c.cloneValue(e.value)
	cloned.Shared = false
	return cloned, true, nil
}

// Set 设置缓存数据
func (c *MemoryCache) Set(ctx context.Context, key Key, value *Value, ttl time.Duration) error {
	if value == nil {
		return fmt.Errorf("cache value cannot be nil")
	}

	now := time.Now()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	value.ExpiresAt = now.Add(ttl)

	mapKey := c.makeMapKey(key)
	stored := c.cloneValue(value)

	c.mu.Lock()
	defer c.mu.Unlock()

	// 容量控制：超出限制时淘汰最旧的条目
	if c.maxEntries > 0 && len(c.data) >= c.maxEntries {
		c.evictOldestLocked()
	}

	c.data[mapKey] = entry{
		key:   key,
		value: stored,
	}
	return nil
}

// DeleteByAuthKey 按AuthKeyID清空对应租户的缓存
func (c *MemoryCache) DeleteByAuthKey(ctx context.Context, authKeyID uint) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, e := range c.data {
		if e.key.Scope.AuthKeyID == authKeyID {
			delete(c.data, k)
		}
	}
	return nil
}

// DeleteByStyle 按API风格清空对应的缓存
func (c *MemoryCache) DeleteByStyle(ctx context.Context, style string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, e := range c.data {
		if e.key.Scope.Style == style {
			delete(c.data, k)
		}
	}
	return nil
}

// Stats 获取缓存统计信息
func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		Entries:   len(c.data),
		HitCount:  c.hitCount,
		MissCount: c.missCount,
	}
}

// makeMapKey 将结构化Key转为map使用的字符串键
func (c *MemoryCache) makeMapKey(key Key) string {
	s := key.Scope
	return fmt.Sprintf("%d|%s|%s|%s|%t|%s",
		s.AuthKeyID, s.Style, s.Model, s.Mode, s.Stream, key.BodyHash)
}

// evictOldestLocked 淘汰最旧的条目（需要持有写锁）
func (c *MemoryCache) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time

	for k, e := range c.data {
		if oldestKey == "" || e.value.CreatedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.value.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(c.data, oldestKey)
	}
}

// Clone 创建 Value 的深拷贝
func (v *Value) Clone() *Value {
	if v == nil {
		return nil
	}

	clone := &Value{
		StatusCode:    v.StatusCode,
		Header:        cloneHeader(v.Header),
		Body:          make([]byte, len(v.Body)),
		CreatedAt:     v.CreatedAt,
		ExpiresAt:     v.ExpiresAt,
		SourceLogID:   v.SourceLogID,
		Usage:         v.Usage,
		ProviderName:  v.ProviderName,
		ProviderModel: v.ProviderModel,
		Shared:        false, // 克隆后不再共享
	}
	copy(clone.Body, v.Body)
	return clone
}

// cloneValue 深拷贝Value，避免数据共享
func (c *MemoryCache) cloneValue(v *Value) *Value {
	return v.Clone()
}

// cloneHeader 深拷贝http.Header
func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}

	clone := make(http.Header, len(h))
	for k, vv := range h {
		values := make([]string, len(vv))
		copy(values, vv)
		clone[k] = values
	}
	return clone
}