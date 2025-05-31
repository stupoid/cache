package cache

import (
	"testing"
	"time"
	"weak"
)

// Dummy target type for testing Janitor
type mockTarget struct {
	cleaned bool
}

// Dummy cleanup function for testing
func mockCleanupFunc(target *mockTarget) {
	if target != nil {
		target.cleaned = true
	}
}

func TestNewJanitor(t *testing.T) {
	target := &mockTarget{}
	wp := weak.Make(target)
	interval := 100 * time.Millisecond
	cleanup := mockCleanupFunc

	tests := []struct {
		name          string
		wp            weak.Pointer[mockTarget]
		interval      time.Duration
		cleanup       func(*mockTarget)
		expectNil     bool
		checkInstance func(*testing.T, *Janitor[mockTarget], *mockTarget, time.Duration)
	}{
		{
			name:      "ValidArgs",
			wp:        wp,
			interval:  interval,
			cleanup:   cleanup,
			expectNil: false,
			checkInstance: func(t *testing.T, janitor *Janitor[mockTarget], expectedTarget *mockTarget, expectedInterval time.Duration) {
				if janitor.targetWeakRef.Value() != expectedTarget {
					t.Errorf("Expected janitor.targetWeakRef to point to the target instance")
				}
				if janitor.interval != expectedInterval {
					t.Errorf("Expected janitor.interval to be %v, got %v", expectedInterval, janitor.interval)
				}
				if janitor.cleanupFunc == nil {
					t.Errorf("Expected janitor.cleanupFunc to be non-nil")
				}
				if janitor.stop == nil {
					t.Errorf("Expected janitor.stop channel to be initialized")
				}
			},
		},
		{
			name:      "NilCleanupFunc",
			wp:        wp,
			interval:  interval,
			cleanup:   nil,
			expectNil: true,
		},
		{
			name:      "ZeroInterval",
			wp:        wp,
			interval:  0,
			cleanup:   cleanup,
			expectNil: true,
		},
		{
			name:      "NegativeInterval",
			wp:        wp,
			interval:  -5 * time.Millisecond,
			cleanup:   cleanup,
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			janitor := NewJanitor[mockTarget](tc.wp, tc.interval, tc.cleanup)

			if tc.expectNil {
				if janitor != nil {
					t.Errorf("Expected NewJanitor to return nil, got %v", janitor)
				}
			} else {
				if janitor == nil {
					t.Errorf("Expected NewJanitor to return a non-nil instance, got nil")
				} else if tc.checkInstance != nil {
					// For the valid case, we pass the original target and interval for comparison
					// In a more complex scenario, these might also come from the test case struct
					originalTargetForValidCase := target
					originalIntervalForValidCase := interval
					if tc.name == "ValidArgs" { // Ensure we use the correct target and interval for the "ValidArgs" check
						tc.checkInstance(t, janitor, originalTargetForValidCase, originalIntervalForValidCase)
					}
				}
			}
		})
	}
}
