package cloudformation

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
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

// CloudFormationCmd must not re-register the global flag set (--base-path,
// --chdir, --config, --cast, --ai, --mask, --no-color, --profile,
// --profiler-*, --redirect-stderr, --settings-list-merge-strategy, --skill,
// --identity) as its own local persistent flags — those are already
// registered persistently on RootCmd (cmd/root.go) and inherited by every
// subcommand automatically. Duplicating them here (previously via
// flags.WithCommonFlags(), which pulls in the entire
// flags.GlobalFlagsRegistry()) shadowed the global registration with a
// second, separately-viper-bound copy and made `atmos aws cfn --help` show
// every global flag instead of just this command family's own. --stack and
// --dry-run are the two flags this command family genuinely needs locally
// (subcommands read them), matching terraform's own local-flag scope.
func TestCloudFormationCmd_DoesNotDuplicateGlobalFlags(t *testing.T) {
	globalOnlyFlags := []string{
		"base-path", "chdir", "config", "config-path", "cast", "ai",
		"force-color", "force-tty", "heatmap", "heatmap-mode", "logs-file",
		"logs-level", "mask", "no-color", "pager", "profile", "profiler-host",
		"profiler-port", "redirect-stderr", "settings-list-merge-strategy",
		"skill", "edition", "identity",
	}
	for _, name := range globalOnlyFlags {
		assert.Nil(t, CloudFormationCmd.PersistentFlags().Lookup(name),
			"%q must not be locally registered on CloudFormationCmd — it is already a global RootCmd persistent flag", name)
	}

	assert.NotNil(t, CloudFormationCmd.PersistentFlags().Lookup("stack"), "expected --stack to remain a local persistent flag")
	assert.NotNil(t, CloudFormationCmd.PersistentFlags().Lookup("dry-run"), "expected --dry-run to remain a local persistent flag")
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

// The logs operation command must register the logs-only --follow/-f flag,
// defaulting to false; an unrelated operation must not pick it up.
func TestOperationSpecificFlagOptions_Logs_RegistersFollowFlag(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")

	followFlag := logsCmd.Flag("follow")
	require.NotNil(t, followFlag, "expected logs to register --follow")
	assert.Equal(t, "false", followFlag.DefValue)
	assert.Equal(t, "f", followFlag.Shorthand)

	applyCmd := newOperationCommand("apply", subCommandApply, "Create or update the stack")
	assert.Nil(t, applyCmd.Flag("follow"), "--follow must be logs-only")
}

// getOperationFlags must surface logs' --follow flag as a bool, both when set
// and when left at its default.
func TestGetOperationFlags_IncludesFollow(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")
	require.NoError(t, logsCmd.Flags().Set("follow", "true"))

	flags := getOperationFlags(logsCmd)
	assert.Equal(t, true, flags["follow"])

	logsCmdDefault := newOperationCommand("logs", "logs", "Show the combined event log")
	flags = getOperationFlags(logsCmdDefault)
	assert.Equal(t, false, flags["follow"])
}

// validateOperationArgs must reject --follow combined with --chart on logs,
// with a clear (not silently-ignored) error.
func TestValidateOperationArgs_RejectsFollowWithChart(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")
	require.NoError(t, logsCmd.Flags().Set("follow", "true"))
	require.NoError(t, logsCmd.Flags().Set("chart", "true"))

	err := validateOperationArgs(logsCmd, []string{"demo"})
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationLogsFollowChartExclusive)
}

// --labels must be repeatable (like --tags), accumulating across occurrences
// via pflag's StringSlice type, rather than the last one winning.
func TestApplyTagsAndLabelsFlags_LabelsRepeatAccumulates(t *testing.T) {
	applyCmd := newOperationCommand("apply", subCommandApply, "Create or update the stack")
	require.NoError(t, applyCmd.Flags().Set(flagLabels, "cost-center=platform"))
	require.NoError(t, applyCmd.Flags().Set(flagLabels, "compliance=sox"))

	info := &schema.ConfigAndStacksInfo{}
	applyTagsAndLabelsFlags(applyCmd, info)

	assert.Equal(t, map[string]string{"cost-center": "platform", "compliance": "sox"}, info.Labels)
}

// validateOperationArgs must accept --follow alone (no --chart) on logs.
func TestValidateOperationArgs_AcceptsFollowAlone(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")
	require.NoError(t, logsCmd.Flags().Set("follow", "true"))

	err := validateOperationArgs(logsCmd, []string{"demo"})
	require.NoError(t, err)
}

// validateOperationArgs must be a no-op for the --follow/--chart check on
// commands that don't register those flags at all (every verb except logs).
func TestValidateOperationArgs_FollowChartCheckIsNoOpOnOtherCommands(t *testing.T) {
	applyCmd := newOperationCommand("apply", subCommandApply, "Create or update the stack")
	err := validateOperationArgs(applyCmd, []string{"demo"})
	require.NoError(t, err)
}

// validateOperationArgs must reject --include-dependents without --affected —
// graphSelectionForBulk only reads it inside the --affected branch, so passing
// it with --all (or bare) would otherwise silently do nothing.
func TestValidateOperationArgs_RejectsIncludeDependentsWithoutAffected(t *testing.T) {
	applyCmd := newOperationCommand("apply", subCommandApply, "Create or update the stack")
	require.NoError(t, applyCmd.Flags().Set("include-dependents", "true"))
	require.NoError(t, applyCmd.Flags().Set(flagAll, "true"))

	err := validateOperationArgs(applyCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationIncludeDependentsRequiresAffected)
}

// validateOperationArgs must accept --include-dependents when --affected is set.
func TestValidateOperationArgs_AcceptsIncludeDependentsWithAffected(t *testing.T) {
	applyCmd := newOperationCommand("apply", subCommandApply, "Create or update the stack")
	require.NoError(t, applyCmd.Flags().Set("include-dependents", "true"))
	require.NoError(t, applyCmd.Flags().Set(flagAffected, "true"))

	err := validateOperationArgs(applyCmd, nil)
	require.NoError(t, err)
}
