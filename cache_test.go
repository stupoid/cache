package cache

import (
	"runtime"
	"sync" // Added for concurrent test
	"testing"
	"time"
	"weak"
)

type DummyType struct {
	ID string
}

func TestCacheNoCleanup(t *testing.T) {
	cache := New[*DummyType](0, time.Minute)
	item := &DummyType{ID: "test"}

	cache.Set("key1", item)

	if val, found := cache.Get("key1"); !found || val.ID != "test" {
		t.Errorf("Expected to find item with ID 'test', got %v", val)
	}

	if _, found := cache.Get("key2"); found {
		t.Errorf("Expected to not find item with key 'key2'")
	}
}

func TestCacheCleanup(t *testing.T) {
	cache := New[*DummyType](0, time.Millisecond*100)
	item := &DummyType{ID: "test"}

	cache.Set("key1", item)

	if val, found := cache.Get("key1"); !found || val.ID != "test" {
		t.Errorf("Expected to find item with ID 'test', got %v", val)
	}

	time.Sleep(time.Millisecond * 150)

	cache.cleanup()
	if _, found := cache.Get("key1"); found {
		t.Errorf("Expected item with key 'key1' to be cleaned up")
	}
}

func TestCacheWithCleanup(t *testing.T) {
	cache := New[*DummyType](time.Millisecond*100, time.Millisecond*50)
	item := &DummyType{ID: "test"}

	cache.Set("key1", item)

	if val, found := cache.Get("key1"); !found || val.ID != "test" {
		t.Errorf("Expected to find item with ID 'test', got %v", val)
	}

	time.Sleep(time.Millisecond * 150) // Wait for cleanup to potentially remove the item

	if _, found := cache.Get("key1"); found {
		t.Errorf("Expected item with key 'key1' to be cleaned up")
	}
}

func TestCacheTickerGoroutineStops(t *testing.T) {
	cleanupInterval := time.Millisecond * 100
	defaultExpiration := time.Millisecond * 50

	var wp weak.Pointer[Cache[*DummyType]]

	createCacheAndGetWeakPtr := func() {
		cacheInstance := New[*DummyType](cleanupInterval, defaultExpiration)
		cacheInstance.Set("testKey", &DummyType{ID: "testValue"})
		wp = weak.Make(cacheInstance)
		return
	}

	createCacheAndGetWeakPtr()

	if wp.Value() == nil {
		t.Errorf("Weak pointer should resolve to a non-nil Cache instance after creation.")
	}

	var cacheValue *Cache[*DummyType]
	for range 10 {
		runtime.GC()
		time.Sleep(time.Millisecond * 50)
		cacheValue = wp.Value()
		if cacheValue == nil {
			break
		}
	}

	if cacheValue != nil {
		t.Errorf("Cache instance should have been garbage collected, but weak pointer still resolves.")
	}
}

func TestCacheSetOverwrite(t *testing.T) {
	cache := New[*DummyType](0, time.Minute)
	item1 := &DummyType{ID: "item1"}
	item2 := &DummyType{ID: "item2"}

	cache.Set("key1", item1)
	val, found := cache.Get("key1")
	if !found || val.ID != "item1" {
		t.Errorf("Expected to find item1, got %v", val)
	}

	cache.Set("key1", item2) // Overwrite
	val, found = cache.Get("key1")
	if !found || val.ID != "item2" {
		t.Errorf("Expected to find item2 after overwrite, got %v", val)
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	cache := New[int](time.Millisecond*50, time.Millisecond*20) // Use int for simplicity
	var wg sync.WaitGroup
	numGoroutines := 100
	numOperationsPerGoroutine := 100

	// Concurrent Set operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gNum int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				key := (gNum * numOperationsPerGoroutine) + j
				cache.Set(string(rune(key)), key)
			}
		}(i)
	}
	wg.Wait()

	for i := range numGoroutines {
		wg.Add(1)
		go func(gNum int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				key := (gNum * numOperationsPerGoroutine) + j
				// Mix reads and writes
				if j%2 == 0 {
					cache.Get(string(rune(key)))
				} else {
					cache.Set(string(rune(key)), key*2)
				}
			}
		}(i)
	}
	wg.Wait()

	_, found := cache.Get(string(rune(0)))
	t.Logf("Item 0 found: %v (this is informational for concurrent test)", found)
}
