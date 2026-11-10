package main

import (
	"fmt"
)

type LFRCashe struct {
	cap, clock int
	items      map[string]*lfrItem
}

type lfrItem struct {
	val              any
	frequency, stamp int
}

func NewLFRCache(cap int) *LFRCashe {
	if cap <= 0 {
		panic("capacity must be positive")
	}
	return &LFRCashe{cap: cap, items: make(map[string]*lfrItem)}
}

func (c *LFRCashe) Get(key string) (any, bool) {
	val, ok := c.items[key]
	if !ok {
		return nil, false
	}

	c.clock++
	val.frequency++
	val.stamp = c.clock

	return val.val, true
}

func (c *LFRCashe) Put(key string, val any) any {
	v, ok := c.items[key]
	if ok {
		c.clock++
		v.frequency++
		v.stamp = c.clock
		v.val = val

		c.items[key] = v
		return v.val
	}

	c.displace()

	c.clock++
	c.items[key] = &lfrItem{
		val:       val,
		frequency: 1,
		stamp:     c.clock,
	}

	return val
}

func (c *LFRCashe) displace() {
	if len(c.items) < c.cap {
		return
	}
	victim, first := "", true
	var best *lfrItem
	for key, item := range c.items {
		if first || item.frequency < best.frequency ||
			(item.frequency == best.frequency && item.stamp < best.stamp) {
			victim, best, first = key, item, false
		}
	}
	delete(c.items, victim)
}

func main() {
	c := NewLFRCache(2)

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
