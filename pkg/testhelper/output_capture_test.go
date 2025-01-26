package testhelper

import (
	"fmt"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
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
				t.Error("Intentionally failing test to verify output buffering")
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

func TestRunWithConfig(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "test-component",
		Stack:            "test-stack",
		ComponentSection: map[string]any{
			"vars":     map[string]any{"env": "test"},
			"metadata": map[string]any{"name": "test"},
		},
	}

	tests := []struct {
		name          string
		fn            func(*testing.T, schema.ConfigAndStacksInfo)
		disableBuffer bool
	}{
		{
			name: "captures output with config",
			fn: func(t *testing.T, info schema.ConfigAndStacksInfo) {
				fmt.Printf("Component: %s, Stack: %s\n", info.ComponentFromArg, info.Stack)
			},
		},
		{
			name: "with disabled buffering",
			fn: func(t *testing.T, info schema.ConfigAndStacksInfo) {
				fmt.Printf("Unbuffered - Component: %s, Stack: %s\n", info.ComponentFromArg, info.Stack)
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
			RunWithConfig(t, configAndStacksInfo, tt.fn)
		})
	}
}
