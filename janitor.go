package cache

import (
	"time"
	"weak"
)

// StartJanitor initiates a background goroutine that periodically calls cleanupFunc
// on the instance pointed to by targetWeakRef, as long as targetWeakRef resolves
// to a non-nil instance.
// The goroutine stops if the target instance is garbage collected.
// T is the underlying type of the object stored in the weak pointer (e.g., Cache[ItemType]).
// cleanupFunc will be called with a pointer to T (e.g., *Cache[ItemType]).
func StartJanitor[T any](targetWeakRef weak.Pointer[T], interval time.Duration, cleanupFunc func(targetInstancePointer *T)) {
	if cleanupFunc == nil || interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				instancePointer := targetWeakRef.Value()
				if instancePointer == nil {
					return
				}
				cleanupFunc(instancePointer)
			}
		}
	}()
}
