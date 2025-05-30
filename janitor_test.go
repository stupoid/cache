package cache

import (
	"testing"
	"time"
	"weak"
)

func TestJanitorInvalidParameters(_ *testing.T) {
	// Test with nil cleanupFunc
	var wpNilFunc weak.Pointer[Cache[*DummyType]]
	cacheInstance := New[*DummyType](0, time.Minute) // A dummy cache instance
	wpNilFunc = weak.Make(cacheInstance)

	// This call should not panic and should return quickly.
	StartJanitor(wpNilFunc, time.Millisecond*10, nil)

	// Test with zero interval
	var wpZeroInterval weak.Pointer[Cache[*DummyType]]
	wpZeroInterval = weak.Make(cacheInstance)
	cleanupFn := func(c *Cache[*DummyType]) { c.cleanup() }

	// This call should not panic and should return quickly.
	StartJanitor(wpZeroInterval, 0, cleanupFn)

	// Test with negative interval
	var wpNegativeInterval weak.Pointer[Cache[*DummyType]]
	wpNegativeInterval = weak.Make(cacheInstance)

	// This call should not panic and should return quickly.
	StartJanitor(wpNegativeInterval, -time.Millisecond*10, cleanupFn)

	time.Sleep(50 * time.Millisecond)
}
