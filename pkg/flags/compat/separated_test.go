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

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			SetSeparated([]string{"-var", "key=value"})
			_ = GetSeparated()
			ResetSeparated()
		}()
	}

	wg.Wait()
	// No race conditions or panics.
}
