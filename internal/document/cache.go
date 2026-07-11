package document

import (
	"container/list"
	"image"
)

// CacheKey identifies a rendered page at a specific DPI.
type CacheKey struct {
	Page int
	DPI  int
}

type cacheEntry struct {
	key CacheKey
	img *image.RGBA
}

// Cache is a small LRU cache of rendered page images, keyed by
// (page index, DPI). It exists to avoid re-rendering the page the user
// just navigated away from and back to.
type Cache struct {
	capacity int
	ll       *list.List // front = most recently used
	items    map[CacheKey]*list.Element
}

// NewCache creates an LRU cache holding at most capacity entries.
func NewCache(capacity int) *Cache {
	return &Cache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[CacheKey]*list.Element),
	}
}

// Get returns the cached image for key, if present, and marks it as
// recently used.
func (c *Cache) Get(key CacheKey) (*image.RGBA, bool) {
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*cacheEntry).img, true
}

// Put stores img under key, evicting the least-recently-used entry if the
// cache is over capacity.
func (c *Cache) Put(key CacheKey, img *image.RGBA) {
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).img = img
		c.ll.MoveToFront(el)
		return
	}

	el := c.ll.PushFront(&cacheEntry{key: key, img: img})
	c.items[key] = el

	for c.ll.Len() > c.capacity {
		oldest := c.ll.Back()
		if oldest == nil {
			break
		}
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}
