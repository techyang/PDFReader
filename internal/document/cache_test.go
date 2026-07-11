package document

import (
	"image"
	"testing"
)

func fakeImage(w, h int) *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

func TestCache_GetMiss(t *testing.T) {
	c := NewCache(3)
	if _, ok := c.Get(CacheKey{Page: 0, DPI: 72}); ok {
		t.Fatal("Get on empty cache should miss")
	}
}

func TestCache_PutThenGet(t *testing.T) {
	c := NewCache(3)
	img := fakeImage(10, 10)
	key := CacheKey{Page: 0, DPI: 72}

	c.Put(key, img)

	got, ok := c.Get(key)
	if !ok {
		t.Fatal("Get after Put should hit")
	}
	if got != img {
		t.Fatal("Get returned a different image than was Put")
	}
}

func TestCache_EvictsOldestWhenOverCapacity(t *testing.T) {
	c := NewCache(2)
	c.Put(CacheKey{Page: 0, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 1, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 2, DPI: 72}, fakeImage(1, 1)) // should evict page 0

	if _, ok := c.Get(CacheKey{Page: 0, DPI: 72}); ok {
		t.Fatal("page 0 should have been evicted")
	}
	if _, ok := c.Get(CacheKey{Page: 1, DPI: 72}); !ok {
		t.Fatal("page 1 should still be cached")
	}
	if _, ok := c.Get(CacheKey{Page: 2, DPI: 72}); !ok {
		t.Fatal("page 2 should be cached")
	}
}

func TestCache_GetRefreshesRecency(t *testing.T) {
	c := NewCache(2)
	c.Put(CacheKey{Page: 0, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 1, DPI: 72}, fakeImage(1, 1))

	c.Get(CacheKey{Page: 0, DPI: 72}) // touch page 0, page 1 becomes least-recent

	c.Put(CacheKey{Page: 2, DPI: 72}, fakeImage(1, 1)) // should evict page 1, not page 0

	if _, ok := c.Get(CacheKey{Page: 1, DPI: 72}); ok {
		t.Fatal("page 1 should have been evicted")
	}
	if _, ok := c.Get(CacheKey{Page: 0, DPI: 72}); !ok {
		t.Fatal("page 0 should still be cached (recently touched)")
	}
}

func TestCache_DifferentDPISameSamePageAreDistinctKeys(t *testing.T) {
	c := NewCache(3)
	c.Put(CacheKey{Page: 0, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 0, DPI: 150}, fakeImage(2, 2))

	img72, ok := c.Get(CacheKey{Page: 0, DPI: 72})
	if !ok || img72.Bounds().Dx() != 1 {
		t.Fatalf("expected distinct cache entry for DPI 72")
	}
	img150, ok := c.Get(CacheKey{Page: 0, DPI: 150})
	if !ok || img150.Bounds().Dx() != 2 {
		t.Fatalf("expected distinct cache entry for DPI 150")
	}
}

func TestCache_PutExistingKeyUpdatesValueWithoutGrowing(t *testing.T) {
	c := NewCache(2)
	key := CacheKey{Page: 0, DPI: 72}
	c.Put(key, fakeImage(1, 1))
	c.Put(CacheKey{Page: 1, DPI: 72}, fakeImage(1, 1))

	newImg := fakeImage(9, 9)
	c.Put(key, newImg) // re-Put on existing key should update, not evict page 1

	got, ok := c.Get(key)
	if !ok || got != newImg {
		t.Fatal("re-Put on existing key should update its value")
	}
	if _, ok := c.Get(CacheKey{Page: 1, DPI: 72}); !ok {
		t.Fatal("updating an existing key should not evict other entries")
	}
}

func TestCache_CapacityOneEvictsPreviousOnEachPut(t *testing.T) {
	c := NewCache(1)
	c.Put(CacheKey{Page: 0, DPI: 72}, fakeImage(1, 1))
	c.Put(CacheKey{Page: 1, DPI: 72}, fakeImage(1, 1))

	if _, ok := c.Get(CacheKey{Page: 0, DPI: 72}); ok {
		t.Fatal("page 0 should have been evicted from a capacity-1 cache")
	}
	if _, ok := c.Get(CacheKey{Page: 1, DPI: 72}); !ok {
		t.Fatal("page 1 should be cached")
	}
}

func TestCache_GetOnEvictedKeyMisses(t *testing.T) {
	c := NewCache(1)
	key := CacheKey{Page: 0, DPI: 72}
	c.Put(key, fakeImage(1, 1))
	c.Put(CacheKey{Page: 1, DPI: 72}, fakeImage(1, 1)) // evicts key

	if got, ok := c.Get(key); ok || got != nil {
		t.Fatal("Get on an evicted key should miss and return nil")
	}
}
