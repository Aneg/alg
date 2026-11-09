package main

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"
)

type lruElement struct {
	key string
	val any
}

type LRUCache struct {
	cap     int
	list    *list.List
	items   map[string]*list.Element
	mx      sync.RWMutex
	touches chan string
}

func NewLRUCache(ctx context.Context, cap int, chanLen int) *LRUCache {
	if cap <= 0 {
		panic("capacity must be positive")
	}

	if chanLen <= 0 {
		panic("chanLen must be positive")
	}
	res := &LRUCache{cap: cap, list: list.New(), items: make(map[string]*list.Element), touches: make(chan string, chanLen)}
	go res.processTouches(ctx)

	return res
}

func (c *LRUCache) Get(key string) (any, bool) {
	c.mx.RLock()
	v, ok := c.items[key]

	defer c.mx.RUnlock()

	if !ok {
		return nil, false
	}

	select {
	case c.touches <- key:
	default:
		// Очерель заполнена. Пропускаем обновления
	}

	// c.list.MoveToFront(v)
	return v.Value.(*lruElement).val, true
}

func (c *LRUCache) Put(key string, val any) {
	c.mx.Lock()
	defer c.mx.Unlock()

	v, ok := c.items[key]
	if ok {
		v.Value = &lruElement{key: key, val: val}
		c.list.MoveToFront(v)
		return
	}

	if c.cap == c.list.Len() {
		old := c.list.Back()
		delete(c.items, old.Value.(*lruElement).key)
		c.list.Remove(old)
	}

	c.items[key] = c.list.PushFront(&lruElement{key: key, val: val})
}

func (c *LRUCache) Len() int {
	c.mx.RLock()
	defer c.mx.RUnlock()
	return len(c.items)
}

func (c *LRUCache) Delete(key string) bool {
	c.mx.Lock()
	defer c.mx.Unlock()

	v, ok := c.items[key]

	if !ok {
		return false
	}

	c.list.Remove(v)
	delete(c.items, key)
	return true
}

func (c *LRUCache) processTouches(ctx context.Context) {
	const (
		batchSize     = 100
		flushInterval = 10 * time.Millisecond
	)

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]string, 0, batchSize)
	lastIndex := make(map[string]int, batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}

		c.mx.Lock()

		for i, key := range batch {
			// Пропускаем все обращения, кроме последнего.
			if lastIndex[key] != i {
				continue
			}

			if el, ok := c.items[key]; ok {
				c.list.MoveToFront(el)
			}
		}

		c.mx.Unlock()

		batch = batch[:0]
		clear(lastIndex)
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return

		case key, ok := <-c.touches:
			if !ok {
				flush()
				return
			}

			batch = append(batch, key)
			lastIndex[key] = len(batch) - 1

			if len(batch) >= batchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	c := NewLRUCache(ctx, 2, 2)

	c.Put("1", 10)
	c.Put("2", 13)
	c.Put("2", 14)
	c.Put("3", 15)

	fmt.Println(c.Get("2"))
	fmt.Println(c.Get("3"))
	fmt.Println(c.Get("1"))

	cancel()
}
