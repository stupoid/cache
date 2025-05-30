// Package cache provides a simple in-memory key-value cache with item expiration and automatic cleanup.
package cache

import (
	"sync"
	"time"
	"weak"
)

// item represents a cached value along with its expiration timestamp.
type item[T any] struct {
	expiration int64 // expiration is the UnixNano timestamp when the item expires.
	value      T     // value is the cached data.
}

// Cache is a thread-safe in-memory key-value store with item expiration.
type Cache[T any] struct {
	items           map[string]item[T] // items stores the cache data.
	cleanupInterval time.Duration      // cleanupInterval is the duration between cleanup cycles.
	itemExpiration  time.Duration      // itemExpiration is the default duration for item validity.

	mu *sync.RWMutex // mu is used for synchronizing access to the cache items.
}

// New creates a new Cache with the specified cleanup interval and default item expiration.
// If cleanupInterval is greater than 0, a janitor goroutine is started to periodically remove expired items.
func New[T any](cleanupInterval time.Duration, defaultExpiration time.Duration) *Cache[T] {
	c := &Cache[T]{
		items:           make(map[string]item[T]),
		cleanupInterval: cleanupInterval,
		itemExpiration:  defaultExpiration,
		mu:              &sync.RWMutex{},
	}

	if cleanupInterval > 0 {
		wp := weak.Make(c)

		StartJanitor(wp, cleanupInterval, func(target *Cache[T]) {
			target.cleanup()
		})
	}
	return c
}

// Set adds an item to the cache with the default expiration time.
// If an item with the same key already exists, it is overwritten.
func (c *Cache[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = item[T]{
		expiration: time.Now().Add(c.itemExpiration).UnixNano(),
		value:      value,
	}
}

// Get retrieves an item from the cache.
// It returns the item's value and true if the item exists and has not expired.
// Otherwise, it returns the zero value for the type and false.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if it, found := c.items[key]; found {
		if it.expiration > time.Now().UnixNano() {
			return it.value, true
		}
	}
	var zero T
	return zero, false
}

// cleanup removes all expired items from the cache.
// This method is called periodically by the janitor goroutine if cleanupInterval is greater than 0.
func (c *Cache[T]) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixNano()
	for key, it := range c.items {
		if it.expiration <= now {
			delete(c.items, key)
		}
	}
}
