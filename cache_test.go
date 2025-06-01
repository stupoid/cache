package cache

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
	"weak"
)

type DummyType struct {
	ID   string
	Data int
}

const (
	testDefaultExpiration = 50 * time.Millisecond
	testCleanupInterval   = 100 * time.Millisecond
	epsilon               = 5 * time.Millisecond
)

// Helper to create a new cache with default test settings for manual cleanup tests
func newTestCache[T any]() *Cache[T] {
	return New[T](testDefaultExpiration, 0)
}

// TestCache_BasicOperations covers fundamental Set and Get functionalities.
func TestCache_BasicOperations(t *testing.T) {
	t.Run("SetAndGet", func(t *testing.T) {
		cache := newTestCache[*DummyType]()
		item := &DummyType{ID: "item1", Data: 100}
		key := "key1"

		cache.Set(key, item)
		retrieved, found := cache.Get(key)

		if !found {
			t.Fatalf("Expected to find item with key '%s', but not found", key)
		}
		if retrieved == nil || retrieved.ID != item.ID || retrieved.Data != item.Data {
			t.Errorf("Retrieved item %+v does not match original item %+v", retrieved, item)
		}
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		cache := newTestCache[*DummyType]()
		_, found := cache.Get("nonexistentkey")
		if found {
			t.Error("Expected not to find item with non-existent key, but it was found")
		}
	})

	t.Run("SetOverwrite", func(t *testing.T) {
		cache := newTestCache[*DummyType]()
		item1 := &DummyType{ID: "item1", Data: 1}
		item2 := &DummyType{ID: "item2", Data: 2}
		key := "key1"

		cache.Set(key, item1)
		retrieved, found := cache.Get(key)
		if !found || retrieved == nil || retrieved.ID != item1.ID {
			t.Fatalf("Expected item1, got %+v", retrieved)
		}

		cache.Set(key, item2) // Overwrite
		retrieved, found = cache.Get(key)
		if !found || retrieved == nil || retrieved.ID != item2.ID {
			t.Errorf("Expected item2 after overwrite, got %+v", retrieved)
		}
	})
}

// TestCache_DefaultExpirationIsUsed tests that the default expiration is applied when Set is called.
func TestCache_DefaultExpirationIsUsed(t *testing.T) {
	defaultExp := 30 * time.Millisecond
	cache := New[string](defaultExp, 0)
	key := "testDefaultExp"
	now := time.Now()

	cache.Set(key, "someValue")

	// Check it's there before default expiration
	_, found := cache.GetAt(key, now.Add(defaultExp-epsilon))
	if !found {
		t.Errorf("Expected item to be found before default expiration")
	}

	// Check it's gone after default expiration
	_, found = cache.GetAt(key, now.Add(defaultExp+epsilon))
	if found {
		t.Errorf("Expected item to be gone after default expiration")
	}
}

// TestCache_Expiration covers item expiration logic using GetAt.
func TestCache_Expiration(t *testing.T) {
	cache := newTestCache[*DummyType]() // Uses testDefaultExpiration (50ms)
	item := &DummyType{ID: "expires", Data: 1}
	key := "keyExpires"
	now := time.Now() // Reference time for consistent checks

	t.Run("SetWithDefaultExpiration_GetAtChecks", func(t *testing.T) {
		cache.Set(key, item) // Uses default 50ms expiration

		// Check immediately
		retrieved, found := cache.GetAt(key, now)
		if !found || retrieved == nil || retrieved.ID != item.ID {
			t.Errorf("Expected to find item immediately after Set, got found=%v, item=%+v", found, retrieved)
		}

		// Check just before expiration
		beforeExpiry := now.Add(testDefaultExpiration - epsilon)
		retrieved, found = cache.GetAt(key, beforeExpiry)
		if !found || retrieved == nil || retrieved.ID != item.ID {
			t.Errorf("Expected to find item just before expiration, got found=%v, item=%+v", found, retrieved)
		}

		// Check at/after expiration
		atOrAfterExpiry := now.Add(testDefaultExpiration + epsilon) // Give a slight margin
		_, found = cache.GetAt(key, atOrAfterExpiry)
		if found {
			t.Errorf("Expected item to be expired at/after expiration time (%v)", atOrAfterExpiry)
		}
	})
}

// TestCache_SetWithExpiry_Variations tests SetWithExpiry with different durations.
func TestCache_SetWithExpiry_Variations(t *testing.T) {
	cache := New[int](1*time.Hour, 0) // Long default, we'll override; no janitor
	now := time.Now()

	t.Run("SpecificShortExpiry", func(t *testing.T) {
		keyShort := "short"
		valShort := 1
		shortDur := 20 * time.Millisecond
		cache.SetWithExpiry(keyShort, valShort, shortDur)

		_, found := cache.GetAt(keyShort, now.Add(shortDur-epsilon))
		if !found {
			t.Errorf("Expected '%s' to be found just before its short expiry", keyShort)
		}
		_, found = cache.GetAt(keyShort, now.Add(shortDur+epsilon))
		if found {
			t.Errorf("Expected '%s' to be not found just after its short expiry", keyShort)
		}
	})

	t.Run("ZeroExpiry (Never Expires)", func(t *testing.T) {
		keyNever := "never"
		valNever := 2
		cache.SetWithExpiry(keyNever, valNever, 0)

		_, found := cache.GetAt(keyNever, now.Add(1000*time.Hour))
		if !found {
			t.Errorf("Expected '%s' with zero expiry to be found far in the future", keyNever)
		}
	})

	t.Run("NegativeExpiry (Immediate Expiry)", func(t *testing.T) {
		keyNegative := "negative"
		valNegative := 3
		negativeDur := -100 * time.Millisecond
		cache.SetWithExpiry(keyNegative, valNegative, negativeDur)

		_, found := cache.GetAt(keyNegative, now) // Check immediately
		if found {
			t.Errorf("Expected '%s' with negative expiry to be not found immediately", keyNegative)
		}
	})
}

// TestCache_ManualCleanup tests the cleanupAt method.
func TestCache_ManualCleanup(t *testing.T) {
	cache := newTestCache[*DummyType]() // 50ms default expiration, no janitor
	item1 := &DummyType{ID: "item1", Data: 1}
	item2 := &DummyType{ID: "item2", Data: 2} // Will not expire in this test
	item3 := &DummyType{ID: "item3", Data: 3} // Will expire

	key1, key2, key3 := "k1", "k2", "k3"
	baseTime := time.Now()

	cache.SetWithExpiry(key1, item1, 50*time.Millisecond) // Expires at baseTime + 50ms
	cache.SetWithExpiry(key2, item2, 0)                   // Never expires
	cache.SetWithExpiry(key3, item3, 10*time.Millisecond) // Expires at baseTime + 10ms

	// Cleanup time: after item3 expires, but before item1 expires
	cleanupTime1 := baseTime.Add(30 * time.Millisecond)
	cache.cleanupAt(cleanupTime1)

	if _, found := cache.GetAt(key1, cleanupTime1); !found {
		t.Errorf("Item %s should not be cleaned up yet", key1)
	}
	if _, found := cache.GetAt(key2, cleanupTime1); !found {
		t.Errorf("Item %s (never expire) should not be cleaned up", key2)
	}
	if _, found := cache.GetAt(key3, cleanupTime1); found {
		t.Errorf("Item %s should have been cleaned up", key3)
	}

	// Cleanup time: after item1 also expires
	cleanupTime2 := baseTime.Add(60 * time.Millisecond)
	cache.cleanupAt(cleanupTime2)

	if _, found := cache.GetAt(key1, cleanupTime2); found {
		t.Errorf("Item %s should have been cleaned up after its expiration", key1)
	}
	if _, found := cache.GetAt(key2, cleanupTime2); !found {
		t.Errorf("Item %s (never expire) should still not be cleaned up", key2)
	}
}

// TestCache_AutomaticCleanup tests the janitor's background cleanup.
func TestCache_AutomaticCleanup(t *testing.T) {
	shortExpiration := 20 * time.Millisecond
	shortCleanupInterval := 40 * time.Millisecond
	cache := New[*DummyType](shortExpiration, shortCleanupInterval) // Janitor enabled

	itemToExpire := &DummyType{ID: "autoExpire", Data: 1}
	keyToExpire := "keyAutoExpire"
	cache.Set(keyToExpire, itemToExpire) // Expires in 20ms

	// Wait for longer than expiration + cleanup interval for janitor to act
	// Item expires at T+20ms. Janitor runs around T+40ms, T+80ms etc.
	// Wait until T + shortExpiration + shortCleanupInterval + epsilon
	time.Sleep(shortExpiration + shortCleanupInterval + epsilon)

	if _, found := cache.Get(keyToExpire); found {
		t.Errorf("Expected item '%s' to be automatically cleaned up by the janitor", keyToExpire)
	}

	itemNeverExpire := &DummyType{ID: "autoNeverExpire", Data: 2}
	keyNeverExpire := "keyAutoNeverExpire"
	cache.SetWithExpiry(keyNeverExpire, itemNeverExpire, 0) // Never expires

	time.Sleep(shortCleanupInterval * 2) // Wait a couple more cycles
	if _, found := cache.Get(keyNeverExpire); !found {
		t.Errorf("Expected item '%s' (never expire) to not be cleaned up by janitor", keyNeverExpire)
	}
}

// TestCache_JanitorLifecycle tests that the janitor goroutine stops when the cache is garbage collected.
func TestCache_JanitorLifecycle(t *testing.T) {
	janitorCleanupInterval := 30 * time.Millisecond
	janitorDefaultExpiration := 20 * time.Millisecond

	var wp weak.Pointer[Cache[*DummyType]]

	func() { // Scope to allow cacheInstance to be eligible for GC
		cacheInstance := New[*DummyType](janitorDefaultExpiration, janitorCleanupInterval)
		cacheInstance.Set("testKey", &DummyType{ID: "testValue"})
		wp = weak.Make(cacheInstance)
	}()

	if wp.Value() == nil {
		t.Fatal("Weak pointer should resolve to non-nil immediately after cache creation scope ends.")
	}

	var cacheValue *Cache[*DummyType]
	maxAttempts := 20
	collected := false
	for range maxAttempts {
		runtime.GC()
		time.Sleep(janitorCleanupInterval/2 + epsilon) // Give time for GC and finalizer
		cacheValue = wp.Value()
		if cacheValue == nil {
			collected = true
			break
		}
	}

	if !collected {
		t.Errorf("Cache instance was not garbage collected after %d attempts, weak pointer still resolves. Janitor might still be running.", maxAttempts)
	} else {
		t.Logf("Cache instance garbage collected. Janitor assumed stopped via finalizer.")
		// Further checks (like NumGoroutine) are possible but can be flaky.
	}
}

// TestCache_Concurrency performs basic concurrent Set and Get operations.
// Run with `go test -race` to detect race conditions.
func TestCache_Concurrency(t *testing.T) {
	cache := New[int](200*time.Millisecond, 50*time.Millisecond) // Janitor enabled
	var wg sync.WaitGroup
	numGoroutines := 50
	numOpsPerGoroutine := 100

	// Concurrent Set operations
	for i := range numGoroutines {
		wg.Add(1)
		go func(gNum int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				key := fmt.Sprintf("con-key-%d-%d", gNum, j)
				value := (gNum * numOpsPerGoroutine) + j
				if j%10 == 0 { // Mix in some SetWithExpiry
					cache.SetWithExpiry(key, value*10, 0) // Never expire
				} else {
					cache.Set(key, value) // Default expiration
				}
			}
		}(i)
	}
	wg.Wait()

	// Concurrent Get operations
	successfulGets := 0
	var mu sync.Mutex // To protect successfulGets counter

	for i := range numGoroutines {
		wg.Add(1)
		go func(gNum int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				key := fmt.Sprintf("con-key-%d-%d", gNum, j)
				// We don't know if it expired or was the "never expire" version
				// Just perform Get to check for races
				if _, found := cache.Get(key); found {
					if j%10 == 0 { // This one should be found (never expires)
						mu.Lock()
						successfulGets++
						mu.Unlock()
					}
				}
			}
		}(i)
	}
	wg.Wait()

	t.Logf("Concurrency test finished. Found %d 'never expire' items. Run with -race flag.", successfulGets)
}

// TestCache_NewNoJanitor verifies cache operation when cleanupInterval is 0.
func TestCache_NewNoJanitor(t *testing.T) {
	cache := New[string](testDefaultExpiration, 0) // cleanupInterval = 0
	key := "noJanitorKey"
	value := "valueForNoJanitor"
	now := time.Now()

	cache.Set(key, value)

	// Item should be retrievable before expiration
	_, found := cache.GetAt(key, now.Add(testDefaultExpiration-epsilon))
	if !found {
		t.Errorf("Item '%s' should be found before expiration even with no janitor", key)
	}

	// Item should NOT be retrievable after expiration (Get handles this)
	_, found = cache.GetAt(key, now.Add(testDefaultExpiration+epsilon))
	if found {
		t.Errorf("Item '%s' should have expired and GetAt should return false, even with no janitor", key)
	}
	// The main point is that the map c.items won't shrink automatically without manual cleanup.
	// This is hard to assert without exposing internal state.
	// The fact that New(exp, 0) works and Get respects expiration is the key check here.
}

// TestCache_Get_TableDriven demonstrates table-driven tests for GetAt operations.
func TestCache_Get_TableDriven(t *testing.T) {
	baseTime := time.Now()
	cache := New[string](100*time.Millisecond, 0) // No janitor

	// Use setWithExpiresAt for precise timing control relative to baseTime
	cache.setWithExpiresAt("k1", "v1", baseTime.Add(50*time.Millisecond))  // Expires at baseTime + 50ms
	cache.setWithExpiresAt("k2", "v2", baseTime.Add(200*time.Millisecond)) // Expires at baseTime + 200ms
	cache.setWithExpiresAt("k3", "v3", time.Time{})                        // Never expires (zero time)

	testCases := []struct {
		name        string
		key         string
		getTime     time.Time // Absolute time for GetAt
		expectFound bool
		expectedVal string
	}{
		{"Get k1 before expiry", "k1", baseTime.Add(40 * time.Millisecond), true, "v1"},
		{"Get k1 at expiry boundary", "k1", baseTime.Add(50 * time.Millisecond), false, ""}, // Expired because item.expiration <= getTime.UnixNano()
		{"Get k1 after expiry", "k1", baseTime.Add(50*time.Millisecond + epsilon), false, ""},
		{"Get k2 before expiry", "k2", baseTime.Add(190 * time.Millisecond), true, "v2"},
		{"Get k2 at expiry boundary", "k2", baseTime.Add(200 * time.Millisecond), false, ""}, // Expired
		{"Get k2 after expiry", "k2", baseTime.Add(200*time.Millisecond + epsilon), false, ""},
		{"Get k3 (never expires) now", "k3", baseTime, true, "v3"},
		{"Get k3 (never expires) future", "k3", baseTime.Add(1000 * time.Hour), true, "v3"},
		{"Get non-existent key", "kx", baseTime, false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, found := cache.GetAt(tc.key, tc.getTime)
			if found != tc.expectFound {
				var itemExpiration int64 = -1
				var itemValue string
				cache.mu.RLock()
				if item, ok := cache.items[tc.key]; ok {
					itemExpiration = item.expiration
					// itemValue = item.value // Be careful with type T here for logging
				}
				cache.mu.RUnlock()

				t.Errorf("Key '%s' (test time: baseTime + %v, GetAt time (UnixNano): %d): Expected found to be %v, but got %v. Item actual expiration (UnixNano): %d. Item value (if exists): %v",
					tc.key, tc.getTime.Sub(baseTime), tc.getTime.UnixNano(), tc.expectFound, found, itemExpiration, itemValue)
			}

			if tc.expectFound {
				if val != tc.expectedVal {
					t.Errorf("Key '%s': Expected value to be '%s', but got '%s'", tc.key, tc.expectedVal, val)
				}
			} else {
				var zeroString string // Zero value for string
				if val != zeroString {
					t.Errorf("Key '%s': Expected value to be zero value for string ('%s'), but got '%s'", tc.key, zeroString, val)
				}
			}
		})
	}
}

// TestCleanupAt_EmptyCache tests cleanupAt on an empty cache.
func TestCleanupAt_EmptyCache(t *testing.T) {
	cache := New[string](testDefaultExpiration, 0)
	cache.cleanupAt(time.Now()) // Should not panic or error
	// No direct way to check len(cache.items) without exposing it,
	// but we can ensure it doesn't affect subsequent operations.
	cache.Set("a", "b")
	if _, found := cache.Get("a"); !found {
		t.Error("Cache should be usable after cleanupAt on empty.")
	}
}

// TestGet_NonExistentKey_ReturnsZeroValue tests that Get returns the zero value for T.
func TestGet_NonExistentKey_ReturnsZeroValue(t *testing.T) {
	t.Run("StringValue", func(t *testing.T) {
		cache := New[string](testDefaultExpiration, 0)
		val, found := cache.Get("nonexistent")
		if found {
			t.Error("Expected found to be false for nonexistent key")
		}
		if val != "" { // Zero value for string
			t.Errorf("Expected zero value for string (''), got '%s'", val)
		}
	})

	t.Run("IntValue", func(t *testing.T) {
		cache := New[int](testDefaultExpiration, 0)
		val, found := cache.Get("nonexistent")
		if found {
			t.Error("Expected found to be false for nonexistent key")
		}
		if val != 0 { // Zero value for int
			t.Errorf("Expected zero value for int (0), got %d", val)
		}
	})

	t.Run("StructPointerValue", func(t *testing.T) {
		cache := New[*DummyType](testDefaultExpiration, 0)
		val, found := cache.Get("nonexistent")
		if found {
			t.Error("Expected found to be false for nonexistent key")
		}
		if val != nil { // Zero value for pointer type
			t.Errorf("Expected zero value for *DummyType (nil), got %+v", val)
		}
	})
}

// TestSet_KeyVariations tests setting items with different key characteristics.
func TestSet_KeyVariations(t *testing.T) {
	cache := New[string](testDefaultExpiration, 0)

	t.Run("EmptyKey", func(t *testing.T) {
		cache.Set("", "emptyKeyValue")
		val, found := cache.Get("")
		if !found {
			t.Error("Expected to find item with empty key")
		}
		if val != "emptyKeyValue" {
			t.Errorf("Expected value 'emptyKeyValue' for empty key, got '%s'", val)
		}
	})

	t.Run("LongKey", func(t *testing.T) {
		// Generate a reasonably long key
		var runes []rune
		for range 256 {
			runes = append(runes, 'a')
		}
		longKey := string(runes) + strconv.FormatInt(time.Now().UnixNano(), 10)

		cache.Set(longKey, "longKeyValue")
		val, found := cache.Get(longKey)
		if !found {
			t.Error("Expected to find item with long key")
		}
		if val != "longKeyValue" {
			t.Errorf("Expected value 'longKeyValue' for long key, got '%s'", val)
		}
	})
}
