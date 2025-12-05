package balancers

import (
	"container/list"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"

	"github.com/samber/lo"
)

type Balancer interface {
	Pop() (uint, error)
	Delete(key uint)
	Reduce(key uint)
}

// 按权重概率抽取，类似抽签。
type Lottery struct {
	mu    sync.RWMutex
	items map[uint]int
}

func NewLottery(items map[uint]int) Balancer {
	return &Lottery{items: items}
}

func (w *Lottery) Pop() (uint, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if len(w.items) == 0 {
		return 0, fmt.Errorf("no provide items")
	}
	total := 0
	for _, v := range w.items {
		total += v
	}
	if total <= 0 {
		return 0, fmt.Errorf("total provide weight must be greater than 0")
	}
	r := rand.IntN(total)
	for k, v := range w.items {
		if r < v {
			return k, nil
		}
		r -= v
	}
	return 0, fmt.Errorf("unexpected error")
}

func (w *Lottery) Delete(key uint) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.items, key)
}

func (w *Lottery) Reduce(key uint) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if weight, ok := w.items[key]; ok {
		dec := weight / 3
		if dec < 1 {
			dec = 1
		}
		w.items[key] = weight - dec
		if w.items[key] <= 0 {
			delete(w.items, key)
		}
	}
}

// 按顺序循环轮转，按权重展开实现真正的加权轮询
type Rotor struct {
	mu      sync.Mutex
	*list.List
	weights map[uint]int
}

func NewRotor(items map[uint]int) *Rotor {
	l := list.New()
	weights := make(map[uint]int)
	entries := lo.Entries(items)
	slices.SortFunc(entries, func(a lo.Entry[uint, int], b lo.Entry[uint, int]) int {
		return b.Value - a.Value
	})
	for _, entry := range entries {
		weights[entry.Key] = entry.Value
		// 按权重展开到队列
		for i := 0; i < entry.Value; i++ {
			l.PushBack(entry.Key)
		}
	}
	return &Rotor{List: l, weights: weights}
}

func (w *Rotor) Pop() (uint, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Len() == 0 {
		return 0, fmt.Errorf("no provide items")
	}
	e := w.Front()
	// 取出队首后移到队尾，实现真正的轮询
	w.MoveToBack(e)
	return e.Value.(uint), nil
}

func (w *Rotor) Delete(key uint) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.weights, key)
	// 移除队列中所有该key的实例
	for e := w.Front(); e != nil; {
		next := e.Next()
		if e.Value.(uint) == key {
			w.Remove(e)
		}
		e = next
	}
}

func (w *Rotor) Reduce(key uint) {
	w.mu.Lock()
	defer w.mu.Unlock()
	weight, ok := w.weights[key]
	if !ok || weight <= 1 {
		w.Delete(key)
		return
	}
	// 降低权重
	w.weights[key] = weight - 1
	// 从队列中移除一个该key的实例
	for e := w.Front(); e != nil; e = e.Next() {
		if e.Value.(uint) == key {
			w.Remove(e)
			return
		}
	}
}
