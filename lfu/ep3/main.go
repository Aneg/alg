package main

import (
	"container/list"
	"fmt"
	"sync"
)

// Группа элементов с одинаковой частотой.
type freqGroup struct {
	freq  int
	items *list.List
}

// Элемент кеша.
type lfuItem struct {
	key string
	val any

	// Узел в списке элементов группы.
	node *list.Element

	// Узел группы в общем списке частот.
	group *list.Element
}

type LFUCache struct {
	mu sync.Mutex

	cap int

	items  map[string]*lfuItem
	groups *list.List
}

func NewLFUCache(capacity int) *LFUCache {
	if capacity <= 0 {
		panic("capacity must be positive")
	}

	return &LFUCache{
		cap:    capacity,
		items:  make(map[string]*lfuItem),
		groups: list.New(),
	}
}

func (c *LFUCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok {
		return nil, false
	}

	c.touch(item)
	return item.val, true
}

func (c *LFUCache) Put(key string, val any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Обновление существующего ключа.
	if item, ok := c.items[key]; ok {
		item.val = val
		c.touch(item)
		return
	}

	if len(c.items) == c.cap {
		c.evict()
	}

	// Новые записи всегда имеют freq = 1.
	var groupNode *list.Element

	first := c.groups.Front()
	if first != nil &&
		first.Value.(*freqGroup).freq == 1 {
		groupNode = first
	} else {
		groupNode = c.groups.PushFront(&freqGroup{
			freq:  1,
			items: list.New(),
		})
	}

	group := groupNode.Value.(*freqGroup)

	item := &lfuItem{
		key:   key,
		val:   val,
		group: groupNode,
	}

	item.node = group.items.PushFront(item)
	c.items[key] = item
}

// Увеличивает частоту на 1.
// Вызывается под c.mu.Lock().
func (c *LFUCache) touch(item *lfuItem) {
	oldGroupNode := item.group
	oldGroup := oldGroupNode.Value.(*freqGroup)

	nextFreq := oldGroup.freq + 1

	// Ищем соседнюю группу частоты freq+1.
	nextGroupNode := oldGroupNode.Next()

	if nextGroupNode == nil ||
		nextGroupNode.Value.(*freqGroup).freq != nextFreq {

		nextGroupNode = c.groups.InsertAfter(
			&freqGroup{
				freq:  nextFreq,
				items: list.New(),
			},
			oldGroupNode,
		)
	}

	// Убираем элемент из старой группы.
	oldGroup.items.Remove(item.node)

	// Добавляем в новую группу как MRU.
	newGroup := nextGroupNode.Value.(*freqGroup)
	item.node = newGroup.items.PushFront(item)
	item.group = nextGroupNode

	// Удаляем пустую группу.
	if oldGroup.items.Len() == 0 {
		c.groups.Remove(oldGroupNode)
	}
}

// Удаляет LFU, а при одинаковой частоте — LRU.
// Вызывается под c.mu.Lock().
func (c *LFUCache) evict() {
	first := c.groups.Front()
	if first == nil {
		return
	}

	group := first.Value.(*freqGroup)

	// Последний элемент — LRU внутри minFreq.
	node := group.items.Back()
	item := node.Value.(*lfuItem)

	group.items.Remove(node)
	delete(c.items, item.key)

	if group.items.Len() == 0 {
		c.groups.Remove(first)
	}
}

func (c *LFUCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
	if !ok {
		return false
	}

	groupNode := item.group
	group := groupNode.Value.(*freqGroup)

	group.items.Remove(item.node)
	delete(c.items, key)

	if group.items.Len() == 0 {
		c.groups.Remove(groupNode)
	}

	return true
}

func (c *LFUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.items)
}

func main() {
	c := NewLFUCache(3)

	c.Put("A", 10)
	c.Put("B", 20)
	c.Put("C", 30)

	c.Get("A") // freq(A) = 2
	c.Get("A") // freq(A) = 3
	c.Get("B") // freq(B) = 2

	// freq=1: C
	// freq=2: B
	// freq=3: A

	c.Delete("C")

	// Теперь минимальная частота = 2.
	// Ничего пересчитывать не нужно.

	c.Put("D", 40)
	c.Put("E", 50) // D вытесняется (freq=1).

	fmt.Println(c.Get("A")) // 10 true
	fmt.Println(c.Get("B")) // 20 true
	fmt.Println(c.Get("D")) // <nil> false
	fmt.Println(c.Get("E")) // 50 true
}
