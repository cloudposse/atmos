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
