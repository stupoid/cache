package cache

import (
	"time"
	"weak"
)

type Janitor[T any] struct {
	targetWeakRef weak.Pointer[T]                // Weak reference to the target instance
	interval      time.Duration                  // Interval at which to call the cleanup function
	cleanupFunc   func(targetInstancePointer *T) // Function to call for cleanup
	stop          chan struct{}                  // Channel to signal stopping the janitor
}

func NewJanitor[T any](targetWeakRef weak.Pointer[T], interval time.Duration, cleanupFunc func(targetInstancePointer *T)) *Janitor[T] {
	if cleanupFunc == nil || interval <= 0 {
		return nil
	}

	return &Janitor[T]{
		targetWeakRef: targetWeakRef,
		interval:      interval,
		cleanupFunc:   cleanupFunc,
		stop:          make(chan struct{}),
	}
}

func (j *Janitor[T]) Start() {
	if j.cleanupFunc == nil || j.interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				instancePointer := j.targetWeakRef.Value()
				if instancePointer == nil {
					return
				}
				j.cleanupFunc(instancePointer)
			case <-j.stop:
				return
			}
		}
	}()
}

func (j *Janitor[T]) Stop() {
	j.stop <- struct{}{}
}
