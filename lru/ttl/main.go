package main

import (
	"container/heap"
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"
)

type lruElement struct {
	expiresAt time.Time
	heapIndex int
	key       string
	val       any
}

// Min-heap по expiresAt.
type expiryHeap []*lruElement

func (h expiryHeap) Len() int {
	return len(h)
}

func (h expiryHeap) Less(i, j int) bool {
	return h[i].expiresAt.Before(h[j].expiresAt)
}

func (h expiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}

func (h *expiryHeap) Push(x any) {
	item := x.(*lruElement)
	item.heapIndex = len(*h)
	*h = append(*h, item)
}

func (h *expiryHeap) Pop() any {
	old := *h
	n := len(old)

	item := old[n-1]
	old[n-1] = nil
	item.heapIndex = -1

	*h = old[:n-1]
	return item
}

type LRUCache struct {
	cap     int
	ttl     time.Duration
	list    *list.List
	items   map[string]*list.Element
	heap    expiryHeap
	mx      sync.RWMutex
	touches chan *lruElement
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool // Защищён mx; после остановки Get работает синхронно.
}

func NewLRUCache(
	ctx context.Context,
	capacity int,
	chanLen int,
	ttl time.Duration,
) *LRUCache {
	if capacity <= 0 {
		panic("capacity must be positive")
	}
	if chanLen <= 0 {
		panic("chanLen must be positive")
	}
	if ttl <= 0 {
		panic("ttl must be positive")
	}

	ctx, cancel := context.WithCancel(ctx)
	res := &LRUCache{
		cap:     capacity,
		ttl:     ttl,
		list:    list.New(),
		items:   make(map[string]*list.Element),
		touches: make(chan *lruElement, chanLen),
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	heap.Init(&res.heap)

	go res.processTouches(ctx)

	return res
}

func (c *LRUCache) Get(key string) (any, bool) {
	c.mx.RLock()
	if c.stopped {
		c.mx.RUnlock()
		return c.getSynchronous(key)
	}

	el, ok := c.items[key]
	if !ok {
		c.mx.RUnlock()
		return nil, false
	}

	item := el.Value.(*lruElement)

	if !time.Now().Before(item.expiresAt) {
		c.mx.RUnlock()

		// Удаляем, только если запись всё ещё та же
		// и её TTL действительно истёк.
		c.mx.Lock()
		if current, exists := c.items[key]; exists {
			currentItem := current.Value.(*lruElement)

			if currentItem == item &&
				!time.Now().Before(currentItem.expiresAt) {
				c.remove(current)
			}
		}
		c.mx.Unlock()

		return nil, false
	}

	val := item.val

	select {
	case c.touches <- item:
	default:
		// Очередь заполнена: теряем событие чтения, но возвращаем значение.
	}

	c.mx.RUnlock()
	return val, true
}

func (c *LRUCache) getSynchronous(key string) (any, bool) {
	c.mx.Lock()
	defer c.mx.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	item := el.Value.(*lruElement)
	if !time.Now().Before(item.expiresAt) {
		c.remove(el)
		return nil, false
	}
	c.list.MoveToFront(el)
	return item.val, true
}

func (c *LRUCache) Put(key string, val any) {
	c.mx.Lock()
	defer c.mx.Unlock()

	now := time.Now()

	// Сначала удаляем просроченные элементы.
	c.cleanupExpired(now)

	if el, ok := c.items[key]; ok {
		old := el.Value.(*lruElement)
		// Новая запись делает события от предыдущего Put устаревшими.
		item := &lruElement{
			key: key, val: val, expiresAt: now.Add(c.ttl),
			heapIndex: old.heapIndex,
		}
		el.Value = item
		c.heap[item.heapIndex] = item
		old.heapIndex = -1

		// Восстанавливаем порядок в куче.
		heap.Fix(&c.heap, item.heapIndex)

		c.list.MoveToFront(el)
		return
	}

	if c.list.Len() == c.cap {
		c.remove(c.list.Back())
	}

	item := &lruElement{
		key:       key,
		val:       val,
		expiresAt: now.Add(c.ttl),
	}

	el := c.list.PushFront(item)
	c.items[key] = el
	heap.Push(&c.heap, item)
}

func (c *LRUCache) Len() int {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.cleanupExpired(time.Now())
	return len(c.items)
}

func (c *LRUCache) Delete(key string) bool {
	c.mx.Lock()
	defer c.mx.Unlock()

	el, ok := c.items[key]
	if !ok {
		return false
	}

	c.remove(el)
	return true
}

// Вызывается только под эксклюзивным Lock.
// Удаляет запись из map, list и heap.
func (c *LRUCache) remove(el *list.Element) {
	item := el.Value.(*lruElement)

	delete(c.items, item.key)
	c.list.Remove(el)
	heap.Remove(&c.heap, item.heapIndex)
}

// Вызывается только под эксклюзивным Lock.
// Удаляет все элементы с истёкшим TTL.
func (c *LRUCache) cleanupExpired(now time.Time) {
	for c.heap.Len() > 0 {
		item := c.heap[0]

		if now.Before(item.expiresAt) {
			break
		}

		heap.Pop(&c.heap)

		el := c.items[item.key]
		delete(c.items, item.key)
		c.list.Remove(el)
	}
}

// Close останавливает фоновые задачи и ждёт их завершения.
// После Close кэш остаётся доступен; Get обновляет список синхронно.
func (c *LRUCache) Close() {
	c.cancel()
	<-c.done
}

func (c *LRUCache) processTouches(ctx context.Context) {
	defer close(c.done)
	const (
		batchSize     = 100
		flushInterval = 10 * time.Millisecond
	)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]*lruElement, 0, batchSize)
	lastIndex := make(map[*lruElement]int, batchSize)

	// Вызывается только под c.mx.Lock().
	flushLocked := func() {
		now := time.Now()
		for i, item := range batch {
			if lastIndex[item] != i {
				continue
			}
			el, ok := c.items[item.key]
			// Событие относится именно к этой версии записи.
			if ok && el.Value.(*lruElement) == item && now.Before(item.expiresAt) {
				c.list.MoveToFront(el)
			}
		}
		clear(batch) // Не удерживаем ссылки на обработанные записи.
		batch = batch[:0]
		clear(lastIndex)
	}
	appendTouch := func(item *lruElement) {
		batch = append(batch, item)
		lastIndex[item] = len(batch) - 1
	}

	for {
		select {
		case <-ctx.Done():
			// Lock исключает новые отправки из Get на время осушения.
			c.mx.Lock()
			for {
				select {
				case item := <-c.touches:
					appendTouch(item)
					if len(batch) >= batchSize {
						flushLocked()
					}
				default:
					flushLocked()
					c.stopped = true
					c.mx.Unlock()
					return
				}
			}

		case item := <-c.touches:
			appendTouch(item)
			if len(batch) >= batchSize {
				c.mx.Lock()
				flushLocked()
				c.mx.Unlock()
			}

		case <-ticker.C:
			c.mx.Lock()
			flushLocked()
			c.cleanupExpired(time.Now())
			c.mx.Unlock()
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := NewLRUCache(
		ctx,
		2,
		100,
		2*time.Second,
	)

	c.Put("1", 10)
	c.Put("2", 20)

	fmt.Println(c.Get("1")) // 10 true

	time.Sleep(3 * time.Second)

	fmt.Println(c.Get("1")) // <nil> false
	fmt.Println(c.Len())    // 0

	c.Put("3", 30)
	fmt.Println(c.Get("3")) // 30 true
}
