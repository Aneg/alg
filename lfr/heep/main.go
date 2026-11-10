package main

import (
	"container/list"
	"fmt"
	"sync"
)

type LFUCache struct {
	mu      sync.Mutex
	cap     int
	items   map[string]*list.Element
	order   map[int]*list.List
	minFreq int
}

type lfuItem struct {
	key       string
	val       any
	frequency int
}

func NewLFUCache(cap int) *LFUCache {
	if cap <= 0 {
		panic("capacity must be positive")
	}
	return &LFUCache{cap: cap, items: make(map[string]*list.Element), order: make(map[int]*list.List)}
}

func (c *LFUCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	val, ok := c.items[key]
	if !ok {
		return nil, false
	}
	item := val.Value.(*lfuItem)

	c.touch(val)

	return item.val, true
}

func (c *LFUCache) Put(key string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]

	if ok {
		v := el.Value.(*lfuItem)
		v.val = val
		c.touch(el)

		return
	}

	if len(c.items) >= c.cap {
		c.evict()
	}

	if c.order[1] == nil {
		c.order[1] = list.New()
	}

	c.items[key] = c.order[1].PushFront(&lfuItem{val: val, frequency: 1, key: key})
	c.minFreq = 1
}

func (c *LFUCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return false
	}

	item := el.Value.(*lfuItem)
	freq := item.frequency

	l := c.order[freq]
	l.Remove(el)

	// Удаляем из map.
	delete(c.items, key)

	if l.Len() == 0 {
		delete(c.order, freq)

		if c.minFreq == freq {
			c.minFreq = 0

			for f := range c.order {
				if c.minFreq == 0 || f < c.minFreq {
					c.minFreq = f
				}
			}
		}
	}

	return true
}

func (c *LFUCache) evict() {
	minList := c.order[c.minFreq]
	minEl := minList.Back()
	minList.Remove(minEl)

	if minList.Len() == 0 {
		delete(c.order, c.minFreq)
	}

	delete(c.items, minEl.Value.(*lfuItem).key)
}

func (c *LFUCache) touch(item *list.Element) {
	el := item.Value.(*lfuItem)
	oldList := c.order[el.frequency]
	oldList.Remove(item)

	if oldList.Len() == 0 {
		delete(c.order, el.frequency)

		if c.minFreq == el.frequency {
			c.minFreq++
		}
	}

	el.frequency++

	if c.order[el.frequency] == nil {
		c.order[el.frequency] = list.New()
	}

	newItem := c.order[el.frequency].PushFront(el)
	c.items[el.key] = newItem
}

func main() {
	c := NewLFUCache(2)

	c.Put("1", 10)
	c.Put("2", 20)
	_, _ = c.Get("1")

	fmt.Println(c.Get("1")) // 10 true

	c.Put("3", 30)
	fmt.Println(c.Get("3")) // 30 true
	fmt.Println(c.Get("2")) // nil, false
	fmt.Println(c.Get("1")) // 10 true
	c.Put("1", 20)
	fmt.Println(c.Get("1")) // 20 true
}
