package cloudformation

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fmt operation command must register the fmt-only --check flag (via
// operationSpecificFlagOptions's "fmt" case), defaulting to false.
func TestOperationSpecificFlagOptions_Fmt_RegistersCheckFlag(t *testing.T) {
	fmtCmd := newOperationCommand("fmt", "fmt", "Format the local template in place")

	checkFlag := fmtCmd.Flag("check")
	require.NotNil(t, checkFlag, "expected fmt to register --check")
	assert.Equal(t, "false", checkFlag.DefValue)

	// --check is fmt-only: an unrelated operation must not pick it up.
	applyCmd := newOperationCommand("apply", subCommandApply, "Create or update the stack")
	assert.Nil(t, applyCmd.Flag("check"), "--check must be fmt-only")
}

// getOperationFlags must surface fmt's --check flag as a bool, both when set
// and when left at its default.
func TestGetOperationFlags_IncludesCheck(t *testing.T) {
	fmtCmd := newOperationCommand("fmt", "fmt", "Format the local template in place")
	require.NoError(t, fmtCmd.Flags().Set("check", "true"))

	flags := getOperationFlags(fmtCmd)
	assert.Equal(t, true, flags["check"])

	fmtCmdDefault := newOperationCommand("fmt", "fmt", "Format the local template in place")
	flags = getOperationFlags(fmtCmdDefault)
	assert.Equal(t, false, flags["check"])
}

// CloudFormationCmd must mount the fmt subcommand, registered via
// subCommandOperations' "fmt" -> OperationFmt entry (exercised end-to-end
// through cmd registration rather than the internal map directly, since the
// map itself lives in pkg/component/aws/cloudformation and is covered there).
func TestCloudFormationCmd_RegistersFmtSubcommand(t *testing.T) {
	var found *cobra.Command
	for _, sub := range CloudFormationCmd.Commands() {
		if sub.Name() == "fmt" {
			found = sub
		}
	}
	require.NotNil(t, found, "expected `atmos aws cloudformation fmt` to be registered")
	assert.NotNil(t, found.Flag("check"))
}
