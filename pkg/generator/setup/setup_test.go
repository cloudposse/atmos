package setup

import (
	"testing"
)

func TestNewGeneratorContext(t *testing.T) {
	ctx, err := NewGeneratorContext()
	if err != nil {
		t.Fatalf("NewGeneratorContext() failed: %v", err)
	}

	if ctx == nil {
		t.Fatal("NewGeneratorContext() returned nil context")
	}

	if ctx.IOContext == nil {
		t.Error("IOContext is nil")
	}

	if ctx.Terminal == nil {
		t.Error("Terminal is nil")
	}

	if ctx.UI == nil {
		t.Error("UI is nil")
	}
}

func TestNewGeneratorContext_AllComponentsInitialized(t *testing.T) {
	ctx, err := NewGeneratorContext()
	if err != nil {
		t.Fatalf("NewGeneratorContext() failed: %v", err)
	}

	// Verify all components are properly initialized and connected
	// The IOContext should be usable
	if ctx.IOContext == nil {
		t.Fatal("IOContext not initialized")
	}

	// The Terminal should be usable
	if ctx.Terminal == nil {
		t.Fatal("Terminal not initialized")
	}

	// The UI should be usable
	if ctx.UI == nil {
		t.Fatal("UI not initialized")
	}
}
