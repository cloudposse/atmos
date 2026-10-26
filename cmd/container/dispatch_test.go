package container

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"

	// Register the container component provider so runVerb's MustGetProvider
	// lookup resolves (it is blank-imported from cmd/root.go in the real binary).
	_ "github.com/cloudposse/atmos/pkg/component/container"
)

// withViperStack sets the viper stack key for the duration of a test and restores
// it afterwards, so global viper state does not leak across tests.
func withViperStack(t *testing.T, stack string) {
	t.Helper()
	v := viper.GetViper()
	orig := v.GetString("stack")
	t.Cleanup(func() { v.Set("stack", orig) })
	v.Set("stack", stack)
}

func TestBuildConfigAndStacksInfo_StackAndDryRun(t *testing.T) {
	withViperStack(t, "dev")

	c := &cobra.Command{Use: "up"}
	c.Flags().Bool("dry-run", false, "")
	require.NoError(t, c.Flags().Set("dry-run", "true"))

	info := buildConfigAndStacksInfo(c)
	assert.Equal(t, "dev", info.Stack)
	assert.True(t, info.DryRun)
}

func TestBuildConfigAndStacksInfo_NoStackNoDryRun(t *testing.T) {
	withViperStack(t, "")

	info := buildConfigAndStacksInfo(&cobra.Command{Use: "up"})
	assert.Empty(t, info.Stack)
	assert.False(t, info.DryRun)
}

func TestBuildConfigAndStacksInfo_TagsAndLabels(t *testing.T) {
	c := newBulkVerbCmd("up", "Up", "Up")
	require.NoError(t, c.Flags().Set("tags", "production,tier-1"))
	require.NoError(t, c.Flags().Set("labels", "cost-center=platform, compliance = sox"))

	info := buildConfigAndStacksInfo(c)
	assert.Equal(t, []string{"production", "tier-1"}, info.Tags)
	assert.Equal(t, map[string]string{"cost-center": "platform", "compliance": "sox"}, info.Labels)
}

func TestBuildConfigAndStacksInfo_NoTagsOrLabelsFlag(t *testing.T) {
	// A verb without withAllFlag (e.g. exec) never registers --tags/--labels;
	// buildConfigAndStacksInfo must be nil-safe.
	info := buildConfigAndStacksInfo(&cobra.Command{Use: "exec"})
	assert.Empty(t, info.Tags)
	assert.Empty(t, info.Labels)
}

func TestBuildVerbCmd_RejectsMalformedLabels(t *testing.T) {
	c := newBulkVerbCmd("up", "Up", "Up")
	require.NoError(t, c.Flags().Set("labels", "not-valid"))
	require.Error(t, c.Args(c, nil))
}

func TestInitConfigAndStacksInfo_ComponentOnly(t *testing.T) {
	withViperStack(t, "dev")

	c := &cobra.Command{Use: "ps"}
	require.NoError(t, c.ParseFlags([]string{"api"}))

	info := initConfigAndStacksInfo(c, "ps", c.Flags().Args())
	assert.Equal(t, "api", info.ComponentFromArg)
	assert.Empty(t, info.AdditionalArgsAndFlags)
	assert.Equal(t, cfg.ContainerComponentType, info.ComponentType)
	assert.Equal(t, "ps", info.SubCommand)
	assert.Equal(t, []string{"container", "ps"}, info.CliArgs)
}

func TestInitConfigAndStacksInfo_ExtraPositional(t *testing.T) {
	c := &cobra.Command{Use: "ps"}
	require.NoError(t, c.ParseFlags([]string{"api", "extra"}))

	info := initConfigAndStacksInfo(c, "ps", c.Flags().Args())
	assert.Equal(t, "api", info.ComponentFromArg)
	assert.Equal(t, []string{"extra"}, info.AdditionalArgsAndFlags)
}

func TestInitConfigAndStacksInfo_PassthroughAfterDash(t *testing.T) {
	// `exec api -- sh -lc echo`: component before "--", command after.
	c := &cobra.Command{Use: "exec"}
	require.NoError(t, c.ParseFlags([]string{"api", "--", "sh", "-lc", "echo"}))

	info := initConfigAndStacksInfo(c, "exec", c.Flags().Args())
	assert.Equal(t, "api", info.ComponentFromArg)
	assert.Equal(t, []string{"sh", "-lc", "echo"}, info.AdditionalArgsAndFlags)
}

func TestInitConfigAndStacksInfo_NoArgs(t *testing.T) {
	c := &cobra.Command{Use: "list"}
	require.NoError(t, c.ParseFlags([]string{}))

	info := initConfigAndStacksInfo(c, "list", c.Flags().Args())
	assert.Empty(t, info.ComponentFromArg)
	assert.Empty(t, info.AdditionalArgsAndFlags)
}

func TestRunVerb_DispatchesToProvider(t *testing.T) {
	// Pointed at an empty directory there is no stack to operate on, so the
	// provider's Execute returns an error — but runVerb itself dispatches through
	// the registered container provider (exercising its full path).
	withViperStack(t, "")
	withViperBasePath(t, t.TempDir())

	c := &cobra.Command{Use: "ps"}
	require.NoError(t, c.ParseFlags([]string{"api"}))

	require.Error(t, runVerb(c, "ps", c.Flags().Args()))
}
