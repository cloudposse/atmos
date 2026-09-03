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

// CloudFormationCmd must mount the stackset verb group with its four
// subcommands (create/update/delete/instances).
func TestCloudFormationCmd_RegistersStackSetSubcommand(t *testing.T) {
	var found *cobra.Command
	for _, sub := range CloudFormationCmd.Commands() {
		if sub.Name() == "stackset" {
			found = sub
		}
	}
	require.NotNil(t, found, "expected `atmos aws cloudformation stackset` to be registered")

	names := make([]string, 0, len(found.Commands()))
	for _, sub := range found.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"create", "update", "delete", "instances"}, names)
}

// CloudFormationCmd must mount tree/logs/watch as top-level subcommands.
func TestCloudFormationCmd_RegistersObservabilitySubcommands(t *testing.T) {
	names := make([]string, 0, len(CloudFormationCmd.Commands()))
	for _, sub := range CloudFormationCmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "tree")
	assert.Contains(t, names, "logs")
	assert.Contains(t, names, "watch")
}

// newStackSetCmd's create/update subcommands must register --auto-approve
// (defaulting false, unlike deploy) and --target; delete must register only
// --auto-approve; instances must register neither.
func TestNewStackSetCmd_RegistersFlags(t *testing.T) {
	cmd := newStackSetCmd()

	var create, update, del, instances *cobra.Command
	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "create":
			create = sub
		case "update":
			update = sub
		case "delete":
			del = sub
		case "instances":
			instances = sub
		}
	}
	require.NotNil(t, create)
	require.NotNil(t, update)
	require.NotNil(t, del)
	require.NotNil(t, instances)

	for _, sub := range []*cobra.Command{create, update} {
		autoApprove := sub.Flag(flagAutoApprove)
		require.NotNil(t, autoApprove, "%s must register --auto-approve", sub.Name())
		assert.Equal(t, "false", autoApprove.DefValue)
		assert.NotNil(t, sub.Flag("target"), "%s must register --target", sub.Name())
	}

	deleteAutoApprove := del.Flag(flagAutoApprove)
	require.NotNil(t, deleteAutoApprove, "delete must register --auto-approve")
	assert.Nil(t, del.Flag("target"), "delete must not register --target")

	assert.Nil(t, instances.Flag(flagAutoApprove), "instances is read-only and must not register --auto-approve")
	assert.Nil(t, instances.Flag("target"), "instances must not register --target")
}

// operationSpecificFlagOptions must delegate stackset/observability
// operations to phase3FlagOptions, registering "logs"'s --chart flag
// (defaulting false) and nothing extra for an unrecognized subCommand.
func TestOperationSpecificFlagOptions_DelegatesToPhase3(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")
	chartFlag := logsCmd.Flag("chart")
	require.NotNil(t, chartFlag, "expected logs to register --chart")
	assert.Equal(t, "false", chartFlag.DefValue)

	// --chart is logs-only: an unrelated operation must not pick it up.
	treeCmd := newOperationCommand("tree", "tree", "Render the nested-stack dependency tree")
	assert.Nil(t, treeCmd.Flag("chart"), "--chart must be logs-only")
}

// phase3FlagOptions must return nil (no extra flags) for a subCommand it
// doesn't recognize, keeping tree/watch flag-free beyond the shared set.
func TestPhase3FlagOptions_UnrecognizedSubCommand(t *testing.T) {
	assert.Nil(t, phase3FlagOptions("tree"))
	assert.Nil(t, phase3FlagOptions("watch"))
	assert.Nil(t, phase3FlagOptions("not-a-real-subcommand"))
}

// getOperationFlags must surface logs' --chart flag as a bool, both when set
// and when left at its default.
func TestGetOperationFlags_IncludesChart(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")
	require.NoError(t, logsCmd.Flags().Set("chart", "true"))

	flags := getOperationFlags(logsCmd)
	assert.Equal(t, true, flags["chart"])

	logsCmdDefault := newOperationCommand("logs", "logs", "Show the combined event log")
	flags = getOperationFlags(logsCmdDefault)
	assert.Equal(t, false, flags["chart"])
}
