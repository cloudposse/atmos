package context

import (
	"testing"
	"time"
)

func TestDiscoveryCache_GetSet(t *testing.T) {
	cache := NewDiscoveryCache(1 * time.Second)

	// Initially empty.
	if result := cache.Get(); result != nil {
		t.Error("Expected nil from empty cache")
	}

	// Set and retrieve.
	expected := &DiscoveryResult{
		Files:     []*DiscoveredFile{{Path: "test.go"}},
		TotalSize: 100,
	}
	cache.Set(expected)

	if result := cache.Get(); result == nil {
		t.Error("Expected result from cache")
	} else if result.TotalSize != expected.TotalSize {
		t.Errorf("Expected TotalSize %d, got %d", expected.TotalSize, result.TotalSize)
	}
}

func TestDiscoveryCache_TTL(t *testing.T) {
	cache := NewDiscoveryCache(100 * time.Millisecond)

	// Set result.
	result := &DiscoveryResult{
		Files:     []*DiscoveredFile{{Path: "test.go"}},
		TotalSize: 100,
	}
	cache.Set(result)

	// Should be available immediately.
	if cached := cache.Get(); cached == nil {
		t.Error("Expected result from cache")
	}

	// Wait for TTL to expire.
	time.Sleep(150 * time.Millisecond)

	// Should be expired.
	if cached := cache.Get(); cached != nil {
		t.Error("Expected nil from expired cache")
	}
}

func TestDiscoveryCache_Invalidate(t *testing.T) {
	cache := NewDiscoveryCache(10 * time.Second)

	// Set result.
	result := &DiscoveryResult{
		Files:     []*DiscoveredFile{{Path: "test.go"}},
		TotalSize: 100,
	}
	cache.Set(result)

	// Should be available.
	if cached := cache.Get(); cached == nil {
		t.Error("Expected result from cache")
	}

	// Invalidate.
	cache.Invalidate()

	// Should be empty.
	if cached := cache.Get(); cached != nil {
		t.Error("Expected nil from invalidated cache")
	}
}

func TestDiscoveryCache_Concurrent(t *testing.T) {
	cache := NewDiscoveryCache(1 * time.Second)

	// Set result.
	result := &DiscoveryResult{
		Files:     []*DiscoveredFile{{Path: "test.go"}},
		TotalSize: 100,
	}
	cache.Set(result)

	// Concurrent reads.
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			if cached := cache.Get(); cached == nil {
				t.Error("Expected result from cache")
			}
			done <- true
		}()
	}

	// Wait for all reads.
	for i := 0; i < 10; i++ {
		<-done
	}
}
