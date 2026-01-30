package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Note: IsPRVersion tests are in version_spec_test.go.

func TestBuildTokenRequiredError(t *testing.T) {
	err := buildTokenRequiredError()
	assert.Error(t, err)
	// The error uses ErrAuthenticationFailed sentinel.
	assert.Contains(t, err.Error(), "authentication")
}

func TestHandlePRArtifactError(t *testing.T) {
	t.Run("generic error returns tool installation error", func(t *testing.T) {
		err := handlePRArtifactError(assert.AnError, 2038)
		assert.Error(t, err)
		// Generic errors are wrapped with ErrToolInstall.
		assert.Contains(t, err.Error(), "tool installation")
	})
}

// Note: Full integration tests for InstallFromPR require:
// - A valid GitHub token
// - Network access
// - A real PR with artifacts
// Those tests should be in a separate integration test file with appropriate
// skip conditions.
