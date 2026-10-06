//go:build windows

package acceptance

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestSubprocessSlotSerializesWindowsCommands(t *testing.T) {
	originalSlots := subprocessSlots
	t.Cleanup(func() { subprocessSlots = originalSlots })
	synctest.Test(t, func(t *testing.T) {
		// Create the semaphore inside the bubble so synctest can observe blocked waiters.
		subprocessSlots = make(chan struct{}, maxConcurrentSubprocesses)
		release, err := acquireSubprocessSlot(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if release != nil {
				release()
			}
		}()

		acquired := make(chan struct{})
		go func() {
			releaseNext, err := acquireSubprocessSlot(t.Context())
			if err != nil {
				t.Error(err)
				return
			}
			defer releaseNext()
			close(acquired)
		}()
		synctest.Wait()
		select {
		case <-acquired:
			t.Fatal("a second Windows subprocess acquired a slot before the first exited")
		default:
		}

		release()
		release = nil
		synctest.Wait()
		select {
		case <-acquired:
		default:
			t.Fatal("the next Windows subprocess did not acquire the released slot")
		}
	})
}

func TestSubprocessSlotCancellationWhileWaiting(t *testing.T) {
	originalSlots := subprocessSlots
	t.Cleanup(func() { subprocessSlots = originalSlots })
	synctest.Test(t, func(t *testing.T) {
		subprocessSlots = make(chan struct{}, maxConcurrentSubprocesses)
		release, err := acquireSubprocessSlot(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer release()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan error, 1)
		go func() {
			releaseNext, err := acquireSubprocessSlot(ctx)
			if releaseNext != nil {
				releaseNext()
			}
			result <- err
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting for a subprocess slot: got %v, want context.Canceled", err)
		}
	})
}
