// Package cache provides a simple in-memory key-value cache with item expiration and automatic cleanup.
package cache

import (
	"runtime"
	"sync"
	"time"
	"weak"
)

// item represents a cached value along with its expiration timestamp.
type item[T any] struct {
	expiration int64 // expiration is the UnixNano timestamp when the item expires.
	value      T     // value is the cached data.
}

// Cache is a generic, thread-safe in-memory key-value store with item expiration.
type Cache[T any] struct {
	items             map[string]item[T] // items stores the cache data.
	cleanupInterval   time.Duration      // cleanupInterval is the duration between cleanup cycles.
	defaultExpiration time.Duration      // itemExpiration is the default duration for item validity.

	mu *sync.RWMutex // mu is used for synchronizing access to the cache items.
}

// New creates a new Cache with the specified default expiration and cleanup interval.
// If cleanupInterval is greater than 0, a background goroutine (janitor) is started
// to automatically remove expired items.
func New[T any](defaultExpiration time.Duration, cleanupInterval time.Duration) *Cache[T] {
	c := &Cache[T]{
		items:             make(map[string]item[T]),
		cleanupInterval:   cleanupInterval,
		defaultExpiration: defaultExpiration,
		mu:                &sync.RWMutex{},
	}

	if cleanupInterval > 0 {
		wp := weak.Make(c)

		janitor := NewJanitor(wp, cleanupInterval, func(targetInstancePointer *Cache[T]) {
			if targetInstancePointer != nil {
				targetInstancePointer.cleanup()
			}
		})
		janitor.Start()
		runtime.SetFinalizer(c, func(cache *Cache[T]) {
			janitor.Stop()
		})
	}
	return c
}

// setWithExpiresAt sets an item in the cache with a specific absolute expiration time.
// If expiresAt is the zero value of time.Time, the item will never expire.
// This method locks the cache.
func (c *Cache[T]) setWithExpiresAt(key string, value T, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var expiration int64
	if expiresAt.IsZero() {
		expiration = 0 // Item never expires
	} else {
		expiration = expiresAt.UnixNano()
	}

	c.items[key] = item[T]{
		expiration: expiration,
		value:      value,
	}
}

// Set adds an item to the cache, replacing any existing item, using the default expiration.
// The expiration is calculated from time.Now().
func (c *Cache[T]) Set(key string, value T) {
	var expiresAt time.Time
	if c.defaultExpiration > 0 {
		expiresAt = time.Now().Add(c.defaultExpiration)
	}
	c.setWithExpiresAt(key, value, expiresAt)
}

// SetWithExpiry adds an item to the cache, replacing any existing item, with a specific expiration duration.
// If the duration d is 0, the item will never expire.
// If d is negative, the item is effectively added as already expired.
// The expiration is calculated from time.Now().
func (c *Cache[T]) SetWithExpiry(key string, value T, d time.Duration) {
	var expiresAt time.Time
	if d != 0 { // If d is 0, the item never expires.
		expiresAt = time.Now().Add(d)
	}
	c.setWithExpiresAt(key, value, expiresAt)
}

// Get retrieves an item from the cache.
// It returns the item's value and true if the item was found and has not expired.
// Otherwise, it returns the zero value for T and false.
// The current time is used to check for expiration.
func (c *Cache[T]) Get(key string) (T, bool) {
	return c.GetAt(key, time.Now())
}

// GetAt retrieves an item from the cache, checking its expiration against the provided 'now' time.
// It returns the item's value and true if the item was found and has not expired relative to 'now'.
// Otherwise, it returns the zero value for T and false.
func (c *Cache[T]) GetAt(key string, now time.Time) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if it, found := c.items[key]; found {
		if it.expiration == 0 || it.expiration > now.UnixNano() {
			return it.value, true
		}
	}
	var zero T
	return zero, false
}

// cleanup removes all expired items from the cache.
// This method is typically called by the background janitor goroutine.
// The current time is used to determine which items have expired.
func (c *Cache[T]) cleanup() {
	c.cleanupAt(time.Now())
}

// cleanupAt removes all items from the cache that have expired as of the given 'now' time.
func (c *Cache[T]) cleanupAt(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// There were some benchmarks that showed that using a for loop with a range
	// is still faster than using a min heap or deque to manage expiration for up to 1M items.
	for key, it := range c.items {
		if it.expiration != 0 && it.expiration <= now.UnixNano() {
			delete(c.items, key)
		}
	}
}
