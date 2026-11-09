package main

import "container/list"

type lruEntry struct {
	key   string
	value int
}
type LRUCache struct {
	capacity int
	list     *list.List
	items    map[string]*list.Element
}

func NewLRU(capacity int) *LRUCache {
	if capacity <= 0 {
		panic("capacity must be positive")
	}
	return &LRUCache{capacity, list.New(), make(map[string]*list.Element)}
}

func (c *LRUCache) Get(key string) (int, bool) {
	node, ok := c.items[key]
	if !ok {
		return 0, false
	}
	c.list.MoveToFront(node)
	return node.Value.(lruEntry).value, true
}

func (c *LRUCache) Put(key string, value int) {
	if node, ok := c.items[key]; ok {
		node.Value = lruEntry{key, value}
		c.list.MoveToFront(node)
		return
	}
	if len(c.items) == c.capacity {
		old := c.list.Back()
		delete(c.items, old.Value.(lruEntry).key)
		c.list.Remove(old)
	}
	c.items[key] = c.list.PushFront(lruEntry{key, value})
}
