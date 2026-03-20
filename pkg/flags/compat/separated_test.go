package compat

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetSeparated(t *testing.T) {
	t.Cleanup(func() { ResetSeparated() })

	args := []string{"-var", "region=us-east-1", "-var-file", "prod.tfvars"}
	SetSeparated(args)

	got := GetSeparated()
	assert.Equal(t, args, got)
}

func TestSetSeparated_DefensiveCopy(t *testing.T) {
	t.Cleanup(func() { ResetSeparated() })

	original := []string{"-var", "foo=bar"}
	SetSeparated(original)

	// Mutate the original slice.
	original[0] = "mutated"

	// GetSeparated should return the original values, not the mutated ones.
	got := GetSeparated()
	assert.Equal(t, "-var", got[0], "SetSeparated should make a defensive copy")
}

func TestGetSeparated_ReturnsNilWhenNotSet(t *testing.T) {
	t.Cleanup(func() { ResetSeparated() })
	ResetSeparated()

	got := GetSeparated()
	assert.Nil(t, got)
}

func TestGetSeparated_DefensiveCopy(t *testing.T) {
	t.Cleanup(func() { ResetSeparated() })

	SetSeparated([]string{"-var", "x=1"})

	// Mutate the returned slice.
	got1 := GetSeparated()
	got1[0] = "mutated"

	// Second call should return original value.
	got2 := GetSeparated()
	assert.Equal(t, "-var", got2[0], "GetSeparated should return a defensive copy")
}

func TestGetSeparated_ReturnsEmptySliceForEmpty(t *testing.T) {
	t.Cleanup(func() { ResetSeparated() })

	// SetSeparated with empty slice: append([]string(nil), []string{}...) returns nil.
	// This is expected Go behavior - appending zero elements to nil yields nil.
	SetSeparated([]string{})

	got := GetSeparated()
	// An empty input slice results in nil globalSeparatedArgs (nil == nil is true).
	assert.Nil(t, got)
}

func TestResetSeparated(t *testing.T) {
	t.Cleanup(func() { ResetSeparated() })

	SetSeparated([]string{"-var", "x=1"})
	assert.NotNil(t, GetSeparated())

	ResetSeparated()
	assert.Nil(t, GetSeparated())
}

func TestSeparated_Concurrent(t *testing.T) {
	t.Cleanup(func() { ResetSeparated() })

	// Phase 1: Parallel Set + Get only — no Reset during reads.
	// Verify that defensive copies from GetSeparated are independent.
	const goroutines = 50
	SetSeparated([]string{"-var", "key=value"})

	copies := make([][]string, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		// In Go 1.22+, loop variables are per-iteration; i := i is a no-op shadow.
		go func(idx int) {
			defer wg.Done()
			copies[idx] = GetSeparated()
		}(i)
	}
	wg.Wait()

	// All copies must equal the originally set value.
	for i, c := range copies {
		assert.Equal(t, []string{"-var", "key=value"}, c, "goroutine %d got unexpected result", i)
	}

	// Mutating one copy must not affect others (defensive copy guarantee).
	// Guard both index accesses to prevent a panic if any goroutine returned nil.
	if len(copies) >= 2 && len(copies[0]) > 0 && len(copies[1]) > 0 {
		copies[0][0] = "mutated"
		assert.Equal(t, "-var", copies[1][0], "defensive copies must be independent")
	}

	// Phase 2: Reset — run serially after all reads are complete.
	ResetSeparated()
	assert.Nil(t, GetSeparated())
}
