package main

import (
	"container/list"
	"fmt"
	"sync"
)

type lruElement struct {
	key string
	val any
}

type LRUCache struct {
	cap   int
	list  *list.List
	items map[string]*list.Element
	mx    sync.RWMutex
}

func NewLRUCache(cap int) *LRUCache {
	if cap <= 0 {
		panic("capacity must be positive")
	}

	return &LRUCache{cap: cap, list: list.New(), items: make(map[string]*list.Element)}
}

func (c *LRUCache) Get(key string) (any, bool) {
	c.mx.Lock()
	defer c.mx.Unlock()
	v, ok := c.items[key]
	if !ok {
		return nil, false
	}

	c.list.MoveToFront(v)
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

func main() {
	c := NewLRUCache(2)

	c.Put("1", 10)
	c.Put("2", 13)
	c.Put("2", 14)
	c.Put("3", 14)

	fmt.Println(c.Get("2"))
	fmt.Println(c.Get("3"))
	fmt.Println(c.Get("1"))
}
