# LRU-кеш

**LRU** (Least Recently Used — _«наименее недавно использованный»_) — это популярный алгоритм кэширования и управления памятью, который удаляет из кэша те элементы, к которым обращались дольше всего назад.

**Контракт.** `Get` возвращает значение и отмечает его как недавно использованное. `Put` добавляет или обновляет значение и тоже обновляет его давность. При добавлении нового ключа в заполненный кэш вытесняется ключ, к которому дольше всего не обращались. Обновление существующего ключа ничего не вытесняет. Ёмкость должна быть положительной и ограничивает число записей, а не объём памяти. 

**Устройство.** Хеш-таблица находит узел двусвязного списка, начало списка означает самый свежий доступ. Удалить известный узел и перенести его в начало можно за `O(1)`. Инвариант: ключи таблицы и списка совпадают, каждый ключ представлен ровно один раз.

В базовой реализации `Get` и `Put` имеют ожидаемую амортизированную сложность `O(1)` по числу записей, память — `O(n)`. Эта оценка предполагает обычную работу хеш-таблицы и не учитывает стоимость хеширования длинных ключей и ожидания блокировок.

**Псевдокод**

```text
ОБЪЕКТ LRUCache(capacity > 0):
    items ← таблица «ключ → узел двусвязного списка»
    list ← пустой список; начало означает самый свежий доступ

    МЕТОД Get(key):
        ЕСЛИ key отсутствует в items: ВЕРНУТЬ (НЕТ, ЛОЖЬ)
        node ← items[key]
        перенести node в начало list
        ВЕРНУТЬ (node.value, ИСТИНА)

    МЕТОД Put(key, value):
        ЕСЛИ key есть в items:
            items[key].value ← value
            перенести items[key] в начало list
            ВЕРНУТЬ
        ЕСЛИ размер(items) = capacity:
            old ← удалить последний узел list
            удалить items[old.key]
        node ← добавить новый узел (key, value) в начало list
        items[key] ← node
```

При ёмкости 2 список изменяется так (слева — самый свежий элемент):

| Операция | Список после операции | Результат |
| --- | --- | --- |
| `Put(A, 1)` | A | Добавили A |
| `Put(B, 2)` | B → A | Добавили B |
| `Get(A)` | A → B | Вернули 1 |
| `Put(C, 3)` | C → A | Вытеснили B |
| `Put(A, 4)` | A → C | Обновили A без вытеснения |

Ниже приведены самостоятельные варианты реализации: их нужно запускать отдельно, добавив объявление `package`. Для всех примеров подойдёт **Go 1.21+**: пакетные варианты используют встроенную функцию `clear`.

**Пример реализации на Go**
```go
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
```
## Конкурентный строгий LRU

Первая реализация подходит для одной горутины или внешней синхронизации. Если к кэшу обращаются несколько горутин, его структуры нужно защищать от конкурентных изменений.

`Get` меняет список давности, поэтому ему нужен эксклюзивный `Lock`, как и `Put` и `Delete`. Только `Len` может использовать `RLock`. Все методы ниже защищены одним `RWMutex`, чтобы изменения таблицы и списка оставались согласованными.

Создавать кэш нужно через конструктор; нулевое значение структуры не готово к работе. Кэш с мьютексом нельзя копировать после начала использования. Блокировка защищает структуры кэша, но не содержимое изменяемых объектов, переданных через `any`: доступ к возвращённым map, slice и указателям вызывающий код синхронизирует самостоятельно.

```go
import (
	"container/list"
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
```

## Приблизительный LRU с пакетным учётом чтений

В строгой версии обращения, меняющие список, выполняются последовательно под блокировкой. Простая замена `Lock` на `RLock` в `Get` создаст гонку: перенос узла — это запись.

Можно изменить контракт: `Get` читает значение под `RLock` и отправляет событие в ограниченную очередь, а отдельная горутина обновляет список пачками. При переполнении очереди событие отбрасывается. Это **приблизительный LRU**, который не гарантирует вытеснение действительно самого давно использованного ключа.

Например, при ёмкости 2 последовательность `Put(A), Put(B), Get(A), Put(C)` может вытеснить A, если обработчик ещё не учёл `Get(A)`. Строгий LRU вытеснил бы B. Другая ситуация: `Get(A), Put(B), обработка Get(A)` делает A самым свежим, хотя последним обращением был `Put(B)`. Дедупликация событий не устраняет эти расхождения.

Внутри пакета оставляем последнее событие для каждой версии записи. Это уменьшает число переносов узлов, но каждое принятое событие всё равно проходит через очередь и обработчик. Вместо timestamp используем идентичность записи: каждое добавление или обновление через `Put` создаёт новый `*lruElement`. Событие со старым указателем не влияет на новую версию ключа. Пока событие хранит указатель, старая запись остаётся достижимой и её адрес не может быть переиспользован для новой записи.

Фоновая работа привязана к контексту времени жизни кэша, а не отдельного запроса. После создания кэша стоит запланировать `defer cache.Close()`. `Close()` отменяет её и ждёт завершения; метод допускает повторные и конкурентные вызовы. При остановке обработчик применяет принятые события из пакета и очереди и переключает `Get` на синхронный перенос узла. Кэш остаётся доступен. Это не восстанавливает историю ранее потерянных чтений. Канал не закрываем: после переключения новые события в него не отправляются. Для ожидания завершения после отмены родительского контекста также вызываем `Close()`.

```go
import (
	"container/list"
	"context"
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
	touches chan *lruElement
	cancel  context.CancelFunc
	done    chan struct{}
	stopped bool // Защищён mx; после остановки Get работает синхронно.
}

func NewLRUCache(ctx context.Context, cap int, chanLen int) *LRUCache {
	if cap <= 0 {
		panic("capacity must be positive")
	}

	if chanLen <= 0 {
		panic("chanLen must be positive")
	}
	ctx, cancel := context.WithCancel(ctx)
	res := &LRUCache{
		cap: cap, list: list.New(), items: make(map[string]*list.Element),
		touches: make(chan *lruElement, chanLen), cancel: cancel,
		done: make(chan struct{}),
	}
	go res.processTouches(ctx)

	return res
}

func (c *LRUCache) Get(key string) (any, bool) {
	c.mx.RLock()
	if c.stopped {
		c.mx.RUnlock()
		return c.getSynchronous(key)
	}
	v, ok := c.items[key]

	defer c.mx.RUnlock()

	if !ok {
		return nil, false
	}

	select {
	case c.touches <- v.Value.(*lruElement):
	default:
		// Очередь заполнена: теряем событие чтения, но возвращаем значение.
	}

	return v.Value.(*lruElement).val, true
}

func (c *LRUCache) getSynchronous(key string) (any, bool) {
	c.mx.Lock()
	defer c.mx.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	item := el.Value.(*lruElement)
	c.list.MoveToFront(el)
	return item.val, true
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
		for i, item := range batch {
			if lastIndex[item] != i {
				continue
			}
			el, ok := c.items[item.key]
			// Событие относится именно к этой версии записи.
			if ok && el.Value.(*lruElement) == item {
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
			c.mx.Unlock()
		}
	}
}
```


В этой версии сохраняется порядок последних принятых событий в пакете, но не общий порядок чтений и синхронных `Put`. Защита от устаревших записей устраняет влияние старой версии ключа на новую, однако не превращает алгоритм в строгий LRU.

Ускорение нужно измерять: канал и фоновая обработка имеют собственную стоимость. `Put` и обработка пакета по-прежнему требуют эксклюзивной блокировки. Более того, ожидающий писатель блокирует новые `RLock`. Интервал 10 мс — период проверки, а не гарантированная верхняя граница задержки обработки. Стоит сравнивать throughput, p95/p99 задержки, hit rate и число потерянных событий при разных долях чтений и записей и распределениях ключей.

## TTL

К приблизительному LRU добавим срок жизни записей. Здесь TTL отсчитывается от последнего `Put`; `Get` его не продлевает. При `now >= expiresAt` запись считается просроченной и не возвращается. `Len` удаляет просроченные записи и возвращает число живых на момент проверки. `Delete` возвращает `true`, если физически удалил запись, даже если её срок уже истёк.

Простой способ очистки — периодически обходить все записи за `O(n)` и дополнительно проверять срок при `Get`. Другой способ — минимальная куча по абсолютному времени `expiresAt`. Она позволяет сразу найти ближайшую к истечению запись; удаление и обновление выполняются за `O(log n)`.

В примере ниже куча хранит ровно одну запись на каждый ключ из таблицы и списка, а `heapIndex` указывает на её позицию. При обновлении заменяем запись и в узле списка, и в куче. Это сохраняет проверку идентичности для отложенных событий.

Очистку выполняем периодически, перед `Put`, в `Len` и при обнаружении просроченного значения в `Get`. После `Close()` или завершения обработчика по контексту периодическая очистка прекращается; остальные проверки остаются, а чтения обновляют LRU синхронно. Неиспользуемые просроченные записи могут занимать память до следующего обращения или освобождения самого кэша.

```go
import (
	"container/heap"
	"container/list"
	"context"
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
```

## Гарантии и сложность

| Вариант | Учёт обращений | TTL | Фоновые задачи |
| --- | --- | --- | --- |
| Строгий LRU | Синхронный; в конкурентной версии под `Lock` | Нет | Нет |
| Приблизительный LRU | Чтения учитываются с задержкой и могут теряться | Нет | Обработка событий |
| Приблизительный LRU с TTL | Те же ограничения порядка | От последнего `Put`, без продления при `Get` | Обработка событий и очистка |

Пусть `n` — число записей, `k` — число удаляемых просроченных записей, `b` — число событий в пакете. Для TTL-версии оценки без ожидания блокировок таковы:

| Операция | Сложность |
| --- | --- |
| `Get` живого или отсутствующего ключа | Ожидаемо `O(1)` |
| `Get` с удалением просроченной записи | `O(log n)` |
| `Put` | `O(log n + k log n)` |
| `Delete` существующего ключа | `O(log n)` |
| `Len` | `O(1 + k log n)` |
| Обработка пакета | Ожидаемо `O(b)` |
| Периодическая очистка | `O(1 + k log n)` |

В формулах с логарифмом предполагаем `n >= 2`; для пустого кэша и одной записи соответствующие операции занимают константное время. Память — `O(n + chanLen + batchSize)`, без учёта размеров ключей и значений. Очередь и пакет временно удерживают ссылки в том числе на уже заменённые записи.

Массовая очистка и пакетные переносы выполняются под эксклюзивной блокировкой и могут увеличивать задержки читателей. При `Close` также приходится ждать текущую работу и обрабатывать оставшуюся очередь. Постоянного времени завершения этот API не обещает.

## Дальнейшие доработки

Можно разделить кэш на шарды, каждый со своим мьютексом и списком. Это может уменьшить конкуренцию за блокировки, но даст LRU внутри каждого шарда, а не глобальный LRU. При фиксированных квотах сумма ёмкостей ограничивает общий размер, однако перегруженный шард может вытеснять записи, пока в другом есть свободные места. Динамическое распределение ёмкости потребует дополнительной координации.

Перед применением полезно проверить:

- Для строгого LRU — порядок вытеснения после `Get` и обновления через `Put`, ёмкость 1, промахи и удаление.
- Для приблизительного — допустимое отклонение от строгого LRU, переполнение очереди, игнорирование событий старой версии и остановку с непустой очередью.
- Для TTL — истечение и продление через `Put`, согласованность map/list/heap после удаления, отсутствие просроченных значений в `Get`.
- Для конкурентных вариантов — `go test -race`, конкурентные `Get`/`Put`/`Len`/`Delete` и повторный `Close`.

Границы TTL лучше проверять с управляемыми часами, а пакетную обработку — с управляемым запуском обработчика: тесты, зависящие только от коротких `Sleep`, могут быть нестабильными.

## Источники

- [Go: sync.RWMutex](https://pkg.go.dev/sync#RWMutex) — правила блокировки читателей и писателей.
- [Go: container/heap](https://pkg.go.dev/container/heap) — контракт и сложность операций кучи.
- [Go 1.21: изменения языка](https://go.dev/doc/go1.21#language) — встроенная функция `clear`.
- [Go: Data Race Detector](https://go.dev/doc/articles/race_detector) — проверка конкурентного доступа.

