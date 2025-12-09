package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// BenchmarkResolutionContextPush benchmarks the Push operation.
func BenchmarkResolutionContextPush(b *testing.B) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	node := DependencyNode{
		Component:    "test-component",
		Stack:        "test-stack",
		FunctionType: "terraform.state",
		FunctionCall: "!terraform.state test-component test-stack output",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ctx.Push(atmosConfig, node)
		ctx.Pop(atmosConfig)
	}
}

// BenchmarkResolutionContextPushPop benchmarks Push followed by Pop.
func BenchmarkResolutionContextPushPop(b *testing.B) {
	atmosConfig := &schema.AtmosConfiguration{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewResolutionContext()
		node := DependencyNode{
			Component:    "test",
			Stack:        "test",
			FunctionType: "test",
			FunctionCall: "test",
		}
		_ = ctx.Push(atmosConfig, node)
		ctx.Pop(atmosConfig)
	}
}

// BenchmarkGetOrCreateResolutionContext benchmarks goroutine-local context retrieval.
func BenchmarkGetOrCreateResolutionContext(b *testing.B) {
	ClearResolutionContext()
	defer ClearResolutionContext()

	// Warm up - create context once.
	_ = GetOrCreateResolutionContext()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetOrCreateResolutionContext()
	}
}

// BenchmarkGetGoroutineID benchmarks goroutine ID extraction.
func BenchmarkGetGoroutineID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getGoroutineID()
	}
}

// BenchmarkResolutionContextClone benchmarks context cloning.
func BenchmarkResolutionContextClone(b *testing.B) {
	ctx := NewResolutionContext()
	atmosConfig := &schema.AtmosConfiguration{}

	// Add some nodes to make cloning more realistic.
	for i := 0; i < 5; i++ {
		node := DependencyNode{
			Component:    "component",
			Stack:        "stack",
			FunctionType: "test",
			FunctionCall: "test",
		}
		_ = ctx.Push(atmosConfig, node)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ctx.Clone()
	}
}

// BenchmarkProcessCustomYamlTagsWithContext benchmarks YAML tag processing with cycle detection.
func BenchmarkProcessCustomYamlTagsWithContext(b *testing.B) {
	atmosConfig := &schema.AtmosConfiguration{}
	ctx := NewResolutionContext()

	input := schema.AtmosSectionMapType{
		"string":  "value",
		"number":  123,
		"nested":  map[string]any{"key": "value"},
		"list":    []any{1, 2, 3},
		"complex": map[string]any{"deep": map[string]any{"nested": "value"}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ProcessCustomYamlTagsWithContext(atmosConfig, input, "test-stack", nil, ctx, nil)
	}
}

// BenchmarkProcessCustomYamlTagsOverhead measures overhead of cycle detection.
func BenchmarkProcessCustomYamlTagsOverhead(b *testing.B) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := schema.AtmosSectionMapType{
		"simple": "value",
	}

	b.Run("WithoutCycleDetection", func(b *testing.B) {
		// Simulate old behavior (direct processing without context).
		for i := 0; i < b.N; i++ {
			result, _ := processNodes(atmosConfig, input, "test-stack", nil, nil)
			_ = result
		}
	})

	b.Run("WithCycleDetection", func(b *testing.B) {
		// New behavior with cycle detection.
		for i := 0; i < b.N; i++ {
			ClearResolutionContext()
			_, _ = ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)
		}
	})
}
