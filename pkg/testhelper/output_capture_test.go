package testhelper

import (
	"fmt"
	"os"
	"testing"
)

func TestOutputCapture(t *testing.T) {
	tests := []struct {
		name          string
		fn            func()
		disableBuffer bool
	}{
		{
			name: "captures stdout only",
			fn: func() {
				fmt.Println("Hello, stdout!")
			},
		},
		{
			name: "captures stderr only",
			fn: func() {
				fmt.Fprintln(os.Stderr, "Hello, stderr!")
			},
		},
		{
			name: "captures both stdout and stderr",
			fn: func() {
				fmt.Println("Hello, stdout!")
				fmt.Fprintln(os.Stderr, "Hello, stderr!")
			},
		},
		{
			name: "with disabled buffering",
			fn: func() {
				fmt.Println("This should not be captured")
			},
			disableBuffer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origDisableBuffering := DisableBuffering
			defer func() {
				DisableBuffering = origDisableBuffering
			}()

			DisableBuffering = tt.disableBuffer
			Run(t, func(t *testing.T) {
				tt.fn()
			})
		})
	}
}

func TestOutputCapture_Concurrent(t *testing.T) {
	const numGoroutines = 10
	done := make(chan bool)

	for i := 0; i < numGoroutines; i++ {
		i := i // capture for goroutine
		go func() {
			Run(t, func(t *testing.T) {
				fmt.Printf("Goroutine %d stdout\n", i)
				fmt.Fprintf(os.Stderr, "Goroutine %d stderr\n", i)
			})
			done <- true
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}
