//go:build mage

package main

import (
	"testing"

	"github.com/magefile/mage/mg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLintChangedPropagatesRepoRootError exercises Changed's real
// composition: mg.SerialDeps(Lint.GoModCheck, Lint.CustomGCL) runs
// Lint.GoModCheck first, and mg.SerialDeps (unlike a plain function call)
// reports a dependency failure by panicking rather than returning an error
// to its caller. This confirms Changed genuinely propagates a repo-root
// resolution failure (fails fast, in whatever form mg.SerialDeps uses) rather
// than silently swallowing it and falling through to Precommit.
func TestLintChangedPropagatesRepoRootError(t *testing.T) {
	t.Chdir(t.TempDir())

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = Lint{}.Changed()
	}()

	require.NotNil(t, recovered, "expected Changed to panic via mg.SerialDeps when the repo root can't be resolved")
	err, ok := recovered.(error)
	require.True(t, ok, "expected the panic value to be an error, got %T: %v", recovered, recovered)
	assert.Contains(t, err.Error(), errMageRepoRootNotFound.Error())
	assert.Equal(t, 1, mg.ExitStatus(err))
}
