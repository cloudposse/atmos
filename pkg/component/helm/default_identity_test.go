package helm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Issue #3: `atmos helm` must resolve the stack's default-identity binding (like `atmos terraform`)
// for cluster operations, instead of requiring an explicit --identity. shouldSetupComponentAuth
// decides whether to set up component auth (which auto-detects the stack default identity) before
// processing stacks: always when an explicit identity was given, and for any cluster operation
// (apply/diff/delete). The offline template render must never trigger auth.
func TestShouldSetupComponentAuth(t *testing.T) {
	tests := []struct {
		name      string
		identity  string
		operation Operation
		want      bool
	}{
		{"apply with no identity resolves default", "", OperationApply, true},
		{"diff with no identity resolves default", "", OperationDiff, true},
		{"delete with no identity resolves default", "", OperationDelete, true},
		{"template with no identity stays offline", "", OperationTemplate, false},
		{"explicit identity always sets up auth (template)", "dev", OperationTemplate, true},
		{"explicit identity always sets up auth (apply)", "dev", OperationApply, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{Identity: tc.identity}
			require.Equal(t, tc.want, shouldSetupComponentAuth(info, tc.operation))
		})
	}
}

func TestOperationRequiresCluster(t *testing.T) {
	require.False(t, operationRequiresCluster(OperationTemplate))
	require.True(t, operationRequiresCluster(OperationDiff))
	require.True(t, operationRequiresCluster(OperationApply))
	require.True(t, operationRequiresCluster(OperationDelete))
}
