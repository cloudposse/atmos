package cloudformation

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunOperation_DryRunChangesetsAndDrift(t *testing.T) {
	for _, operation := range []Operation{
		OperationChangesetCreate, OperationChangesetExecute, OperationChangesetDelete,
		OperationChangesetList, OperationDriftDetect, OperationDriftDescribe,
		OperationGetTemplate, OperationGetPolicy,
	} {
		t.Run(string(operation), func(t *testing.T) {
			assertOperationDryRun(t, operation, &stackSpec{StackName: "vpc"}, nil)
		})
	}
}

// assertOperationDryRun uses an unresolvable credential profile so any attempt
// to initialize AWS fails, and leaves auto-approve unset to catch prompting.
func assertOperationDryRun(t *testing.T, operation Operation, spec *stackSpec, flags map[string]any) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "missing-credentials")
	octx := &opContext{
		Ctx: t.Context(),
		Info: &schema.ConfigAndStacksInfo{
			DryRun: true,
			AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{
				Profile: "dry-run-must-not-authenticate", CredentialsFile: missing, ConfigFile: missing,
			}},
		},
		Flags: flags,
	}
	summary, err := runOperation(octx, operation, spec)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"stack_name": spec.StackName}, summary)
}
