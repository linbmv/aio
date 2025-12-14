package balancers

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// ProviderStats Provider统计信息
type ProviderStats struct {
	ID              uint
	Weight          int
	ErrorCount      int
	TotalRequests   int
	AvgResponseTime time.Duration
	LastErrorTime   time.Time
	ConnectionCount int
	LastUsedTime    time.Time
	mutex           sync.RWMutex
}

// UpdateError 更新错误统计
func (s *ProviderStats) UpdateError() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.ErrorCount++
	s.TotalRequests++
	s.LastErrorTime = time.Now()
}

// UpdateSuccess 更新成功统计
func (s *ProviderStats) UpdateSuccess(responseTime time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.TotalRequests++
	s.LastUsedTime = time.Now()

	// 计算平均响应时间
	if s.AvgResponseTime == 0 {
		s.AvgResponseTime = responseTime
	} else {
		s.AvgResponseTime = (s.AvgResponseTime + responseTime) / 2
	}
}

// GetErrorRate 获取错误率
func (s *ProviderStats) GetErrorRate() float64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	if s.TotalRequests == 0 {
		return 0
	}
	return float64(s.ErrorCount) / float64(s.TotalRequests)
}

// GetScore 获取综合评分（越高越好）
func (s *ProviderStats) GetScore() float64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// 基础权重分
	score := float64(s.Weight)

	// 错误率惩罚
	errorRate := s.GetErrorRate()
	score *= (1.0 - errorRate)

	// 响应时间惩罚
	if s.AvgResponseTime > 0 {
		// 响应时间越长，分数越低
		timeScore := 1000.0 / float64(s.AvgResponseTime.Milliseconds())
		score *= timeScore
	}

	// 连接数惩罚
	if s.ConnectionCount > 10 {
		score *= 0.8
	}

	return score
}

// ErrorAwareBalancer 基于错误率的负载均衡器
type ErrorAwareBalancer struct {
	stats map[uint]*ProviderStats
	mutex sync.RWMutex
}

// NewErrorAwareBalancer 创建错误感知负载均衡器
func NewErrorAwareBalancer(items map[uint]int) Balancer {
	stats := make(map[uint]*ProviderStats)
	for id, weight := range items {
		stats[id] = &ProviderStats{
			ID:     id,
			Weight: weight,
		}
	}
	return &ErrorAwareBalancer{
		stats: stats,
	}
}

// Pop 选择错误率最低的Provider
func (b *ErrorAwareBalancer) Pop() (uint, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if len(b.stats) == 0 {
		return 0, fmt.Errorf("no available providers")
	}

	var bestProvider *ProviderStats
	var bestID uint

	for id, stats := range b.stats {
		if bestProvider == nil {
			bestProvider = stats
			bestID = id
			continue
		}

		// 比较错误率，选择错误率最低的
		if stats.GetErrorRate() < bestProvider.GetErrorRate() {
			bestProvider = stats
			bestID = id
		} else if stats.GetErrorRate() == bestProvider.GetErrorRate() {
			// 错误率相同时，选择权重更高的
			if stats.Weight > bestProvider.Weight {
				bestProvider = stats
				bestID = id
			}
		}
	}

	return bestID, nil
}

// Delete 删除Provider
func (b *ErrorAwareBalancer) Delete(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	delete(b.stats, key)
}

// Reduce 降低Provider权重
func (b *ErrorAwareBalancer) Reduce(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if stats, exists := b.stats[key]; exists {
		stats.UpdateError()
	}
}

// UpdateSuccess 更新成功统计
func (b *ErrorAwareBalancer) UpdateSuccess(key uint, responseTime time.Duration) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if stats, exists := b.stats[key]; exists {
		stats.UpdateSuccess(responseTime)
	}
}

// TraceAwareBalancer 基于响应时间的负载均衡器
type TraceAwareBalancer struct {
	stats map[uint]*ProviderStats
	mutex sync.RWMutex
}

// NewTraceAwareBalancer 创建响应时间感知负载均衡器
func NewTraceAwareBalancer(items map[uint]int) Balancer {
	stats := make(map[uint]*ProviderStats)
	for id, weight := range items {
		stats[id] = &ProviderStats{
			ID:     id,
			Weight: weight,
		}
	}
	return &TraceAwareBalancer{
		stats: stats,
	}
}

// Pop 选择响应时间最短的Provider
func (b *TraceAwareBalancer) Pop() (uint, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if len(b.stats) == 0 {
		return 0, fmt.Errorf("no available providers")
	}

	var bestProvider *ProviderStats
	var bestID uint

	for id, stats := range b.stats {
		if bestProvider == nil {
			bestProvider = stats
			bestID = id
			continue
		}

		// 比较响应时间，选择最快的
		if stats.AvgResponseTime == 0 ||
			(bestProvider.AvgResponseTime > 0 && stats.AvgResponseTime < bestProvider.AvgResponseTime) {
			bestProvider = stats
			bestID = id
		}
	}

	return bestID, nil
}

// Delete 删除Provider
func (b *TraceAwareBalancer) Delete(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	delete(b.stats, key)
}

// Reduce 降低Provider权重
func (b *TraceAwareBalancer) Reduce(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if stats, exists := b.stats[key]; exists {
		stats.Weight -= stats.Weight / 3
		if stats.Weight < 1 {
			stats.Weight = 1
		}
	}
}

// UpdateSuccess 更新成功统计
func (b *TraceAwareBalancer) UpdateSuccess(key uint, responseTime time.Duration) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if stats, exists := b.stats[key]; exists {
		stats.UpdateSuccess(responseTime)
	}
}

// WeightRoundRobinBalancer 加权轮询负载均衡器（改进版）
type WeightRoundRobinBalancer struct {
	items   []*weightedItem
	current int
	mutex   sync.RWMutex
}

type weightedItem struct {
	id              uint
	weight          int
	currentWeight   int
	effectiveWeight int
}

// NewWeightRoundRobinBalancer 创建加权轮询负载均衡器
func NewWeightRoundRobinBalancer(items map[uint]int) Balancer {
	var weightedItems []*weightedItem
	for id, weight := range items {
		if weight > 0 {
			weightedItems = append(weightedItems, &weightedItem{
				id:              id,
				weight:          weight,
				currentWeight:   0,
				effectiveWeight: weight,
			})
		}
	}
	return &WeightRoundRobinBalancer{
		items: weightedItems,
	}
}

// Pop 使用加权轮询算法选择Provider
func (b *WeightRoundRobinBalancer) Pop() (uint, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if len(b.items) == 0 {
		return 0, fmt.Errorf("no available providers")
	}

	var selected *weightedItem
	totalWeight := 0

	for _, item := range b.items {
		item.currentWeight += item.effectiveWeight
		totalWeight += item.effectiveWeight

		if selected == nil || item.currentWeight > selected.currentWeight {
			selected = item
		}
	}

	if selected == nil {
		return 0, fmt.Errorf("no provider selected")
	}

	selected.currentWeight -= totalWeight
	return selected.id, nil
}

// Delete 删除Provider
func (b *WeightRoundRobinBalancer) Delete(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	newItems := make([]*weightedItem, 0, len(b.items))
	for _, item := range b.items {
		if item.id != key {
			newItems = append(newItems, item)
		}
	}
	b.items = newItems
}

// Reduce 降低Provider权重
func (b *WeightRoundRobinBalancer) Reduce(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, item := range b.items {
		if item.id == key {
			item.effectiveWeight = int(math.Max(1, float64(item.effectiveWeight)*0.7))
			break
		}
	}
}

// ConnectionAwareBalancer 基于连接数的负载均衡器
type ConnectionAwareBalancer struct {
	stats map[uint]*ProviderStats
	mutex sync.RWMutex
}

// NewConnectionAwareBalancer 创建连接感知负载均衡器
func NewConnectionAwareBalancer(items map[uint]int) Balancer {
	stats := make(map[uint]*ProviderStats)
	for id, weight := range items {
		stats[id] = &ProviderStats{
			ID:     id,
			Weight: weight,
		}
	}
	return &ConnectionAwareBalancer{
		stats: stats,
	}
}

// Pop 选择连接数最少的Provider
func (b *ConnectionAwareBalancer) Pop() (uint, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if len(b.stats) == 0 {
		return 0, fmt.Errorf("no available providers")
	}

	var bestProvider *ProviderStats
	var bestID uint

	for id, stats := range b.stats {
		if bestProvider == nil {
			bestProvider = stats
			bestID = id
			continue
		}

		// 比较连接数，选择连接数最少的
		if stats.ConnectionCount < bestProvider.ConnectionCount {
			bestProvider = stats
			bestID = id
		} else if stats.ConnectionCount == bestProvider.ConnectionCount {
			// 连接数相同时，选择权重更高的
			if stats.Weight > bestProvider.Weight {
				bestProvider = stats
				bestID = id
			}
		}
	}

	return bestID, nil
}

// Delete 删除Provider
func (b *ConnectionAwareBalancer) Delete(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	delete(b.stats, key)
}

// Reduce 降低Provider权重
func (b *ConnectionAwareBalancer) Reduce(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if stats, exists := b.stats[key]; exists {
		stats.Weight -= stats.Weight / 3
		if stats.Weight < 1 {
			stats.Weight = 1
		}
	}
}

// IncrementConnection 增加连接数
func (b *ConnectionAwareBalancer) IncrementConnection(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if stats, exists := b.stats[key]; exists {
		stats.ConnectionCount++
	}
}

// DecrementConnection 减少连接数
func (b *ConnectionAwareBalancer) DecrementConnection(key uint) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if stats, exists := b.stats[key]; exists {
		if stats.ConnectionCount > 0 {
			stats.ConnectionCount--
		}
	}
}
