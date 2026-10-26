// Package recoverytest installs isolated, pinned retry policies for executor tests.
package recoverytest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/policy/admission"
)

// Install loads a policy selecting the supplied event and predicate for this test.
//
//nolint:lintroller // Test fixture helper.
func Install(t *testing.T, event, rule string) {
	t.Helper()
	const policyFileMode = 0o600
	dir := t.TempDir()
	document := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: retry}\nspec:\n  policies:\n    transient:\n      description: Retry transient failures.\n      on: " + event + "\n      action: retry\n      rule: " + rule + "\n      with: {max_attempts: 3}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "retry.yaml"), []byte(document), policyFileMode))
	t.Setenv("ATMOS_POLICY_DIR", dir)
	finish, err := admission.BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(finish)
}
