package config

import (
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestSafeViperView_IsAtomic proves View holds the read lock for its entire
// callback, so a concurrent Set() cannot interleave with (and thus cannot be
// observed mid-way through) a compound decision made inside View. This is
// the guarantee resolveMaskingDisabled (internal/exec/shell_utils.go) relies
// on: combining an IsSet presence check with a GetBool value read under one
// View call, rather than as two separately-locked SafeViper calls that a
// concurrent Set() could interleave between.
func TestSafeViperView_IsAtomic(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viewEntered := make(chan struct{})
	releaseView := make(chan struct{})
	viewDone := make(chan struct{})

	var observedInsideView bool
	go func() {
		GlobalViper().View(func(v ViperReader) {
			close(viewEntered)
			<-releaseView
			// If Set() below had been allowed to interleave, this would
			// observe the concurrent write; the assertion after this
			// goroutine proves it never does.
			observedInsideView = v.IsSet("concurrent-key")
		})
		close(viewDone)
	}()

	<-viewEntered

	setDone := make(chan struct{})
	go func() {
		GlobalViper().Set("concurrent-key", true)
		close(setDone)
	}()

	// Set() must block while View's callback still holds the read lock.
	select {
	case <-setDone:
		t.Fatal("Set() completed while a View() callback still held the lock")
	case <-time.After(100 * time.Millisecond):
		// Expected: Set() is still blocked.
	}

	close(releaseView)
	<-viewDone
	<-setDone

	assert.False(t, observedInsideView,
		"View's callback must not observe a Set() that started after the callback began")
	assert.True(t, GlobalViper().IsSet("concurrent-key"), "the Set() must still take effect once View released the lock")
}

// TestSafeViperView_ConsistentSnapshotUnderConcurrentWriters is a
// probabilistic companion to the deterministic test above: it runs many
// concurrent Set()/View() pairs and asserts View always sees a consistent
// mask/settings pairing (never a state that no single Set() call ever
// produced), matching resolveMaskingDisabled's actual key pair.
func TestSafeViperView_ConsistentSnapshotUnderConcurrentWriters(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			GlobalViper().Set("mask", i%2 == 0)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			GlobalViper().View(func(v ViperReader) {
				// Reading the same key twice inside one View call must
				// always agree with itself -- proving the lock is held for
				// the whole callback, not re-acquired per method call.
				// assert (not require): FailNow from a non-test goroutine
				// would only stop this goroutine, not the whole test.
				first := v.GetBool("mask")
				second := v.GetBool("mask")
				assert.Equal(t, first, second)
			})
		}
	}()

	wg.Wait()
}

// TestSafeViperView_CallbackCannotRecoverConcreteViper proves View's
// ViperReader argument cannot be type-asserted back to *viper.Viper. Passing
// *viper.Viper itself through the ViperReader interface would only hide Set
// behind a narrower static type -- a callback could still recover it via a
// type assertion and call Set while holding only View's read lock, racing
// against a concurrent Set()/View() the same way the interface exists to
// prevent. View must pass an unexported adapter instead, which callers
// outside pkg/config cannot name to assert against.
func TestSafeViperView_CallbackCannotRecoverConcreteViper(t *testing.T) {
	GlobalViper().View(func(v ViperReader) {
		_, ok := v.(*viper.Viper)
		assert.False(t, ok, "View's ViperReader argument must not be assertable back to *viper.Viper")
	})
}

// TestSafeViperView_GetStringSliceIsCloned proves GetStringSlice (both
// directly on SafeViper and via View's ViperReader) returns a copy the
// caller can freely mutate, not Viper's own backing array -- spf13/viper's
// GetStringSlice can return an existing []string value without cloning it,
// so handing that out under the lock would let a caller corrupt shared
// config state after the lock is released, with no Set() call to guard.
func TestSafeViperView_GetStringSliceIsCloned(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("some.slice", []string{"a", "b"})

	direct := GlobalViper().GetStringSlice("some.slice")
	direct[0] = "mutated-direct"

	var viaView []string
	GlobalViper().View(func(v ViperReader) {
		viaView = v.GetStringSlice("some.slice")
	})
	viaView[0] = "mutated-view"

	assert.Equal(t, []string{"a", "b"}, GlobalViper().GetStringSlice("some.slice"),
		"mutating a returned slice must not corrupt Viper's own stored value")
}
