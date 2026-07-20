package emulator

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

type capturingEmulatorProvider struct{ execution *component.ExecutionContext }

func (p *capturingEmulatorProvider) GetType() string                               { return cfg.EmulatorComponentType }
func (p *capturingEmulatorProvider) GetGroup() string                              { return "test" }
func (p *capturingEmulatorProvider) GetBasePath(*schema.AtmosConfiguration) string { return "" }
func (p *capturingEmulatorProvider) ListComponents(context.Context, string, map[string]any) ([]string, error) {
	return nil, nil
}
func (p *capturingEmulatorProvider) ValidateComponent(map[string]any) error { return nil }
func (p *capturingEmulatorProvider) Execute(execution *component.ExecutionContext) error {
	p.execution = execution
	return nil
}

func (p *capturingEmulatorProvider) GenerateArtifacts(*component.ExecutionContext) error { return nil }

func (p *capturingEmulatorProvider) GetAvailableCommands() []string { return nil }

// restoreViperKey snapshots a single Viper key and restores it after the test.
// Note that cmd.NewTestKit cannot be imported here: cmd/root.go imports
// cmd/emulator, so importing cmd from this package would create a circular
// dependency. These tests build fresh cobra.Command instances rather than
// touching RootCmd, so the only shared global state needing isolation is the
// relevant Viper key.
func restoreViperKey(t *testing.T, key string) {
	t.Helper()
	v := viper.GetViper()
	orig := v.Get(key)
	wasSet := v.IsSet(key)
	t.Cleanup(func() {
		if wasSet {
			v.Set(key, orig)
			return
		}
		viper.Reset()
		require.NoError(t, emulatorParser.BindToViper(viper.GetViper()))
		require.NoError(t, upParser.BindToViper(viper.GetViper()))
		require.NoError(t, resetParser.BindToViper(viper.GetViper()))
	})
}

func TestEmulatorCommandProvider(t *testing.T) {
	provider := &EmulatorCommandProvider{}

	c := provider.GetCommand()
	require.NotNil(t, c)
	assert.Equal(t, "emulator", c.Use)
	assert.Equal(t, []string{"emu"}, c.Aliases)
	assert.Equal(t, "emulator", provider.GetName())
	assert.Equal(t, "Core Stack Commands", provider.GetGroup())
	assert.Nil(t, provider.GetAliases())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.True(t, provider.IsExperimental())
}

func TestEmulatorRootRunEShowsUsage(t *testing.T) {
	// The root emulator command with no subcommand prints usage and returns nil.
	require.NotNil(t, emulatorCmd.RunE)
	var out bytes.Buffer
	emulatorCmd.SetOut(&out)
	emulatorCmd.SetErr(&out)
	t.Cleanup(func() {
		emulatorCmd.SetOut(nil)
		emulatorCmd.SetErr(nil)
	})
	require.NoError(t, emulatorCmd.RunE(emulatorCmd, []string{}))
	assert.Contains(t, out.String(), "Usage:")
	assert.Contains(t, out.String(), "emulator [command]")
}

func TestEmulatorCommandStructure(t *testing.T) {
	subcommands := emulatorCmd.Commands()
	names := make([]string, len(subcommands))
	for i, c := range subcommands {
		names[i] = c.Name()
	}
	for _, want := range []string{"up", "down", "reset", "ps", "logs", "exec"} {
		assert.Contains(t, names, want)
	}
}

func TestEmulatorSubcommandsHaveRunEAndValidator(t *testing.T) {
	wantSubcommands := map[string]bool{
		"up": true, "down": true, "reset": true, "logs": true, "exec": true,
	}
	seen := map[string]bool{}
	for _, c := range emulatorCmd.Commands() {
		if !wantSubcommands[c.Name()] {
			continue
		}
		seen[c.Name()] = true
		require.NotNil(t, c.RunE, "subcommand %q should have a RunE", c.Name())
		require.NotNil(t, c.Args, "subcommand %q should have a positional validator", c.Name())
		// Prompt-aware validation lets RunE ask for a missing component before
		// the parser enforces the required argument.
		require.NoError(t, c.Args(c, []string{}), "subcommand %q should defer a missing component to the prompt flow", c.Name())
		require.NoError(t, c.Args(c, []string{"aws"}), "subcommand %q should accept one component", c.Name())
		require.Error(t, c.Args(c, []string{"aws", "extra"}), "subcommand %q should reject extra positional arguments", c.Name())
	}
	for name := range wantSubcommands {
		assert.True(t, seen[name], "expected subcommand %q to be registered", name)
	}
}

func TestEmulatorInspectionVerbsTakeNoComponent(t *testing.T) {
	for _, c := range []*cobra.Command{listCmd, psCmd} {
		require.NoError(t, c.Args(c, []string{}), "%s should not require a component", c.Name())
		require.Error(t, c.Args(c, []string{"aws"}), "%s should reject a component", c.Name())
		require.NotNil(t, c.Flags().Lookup(flagRuntime), "%s should support --runtime", c.Name())
	}
}

func TestExecWhitelistsUnknownFlags(t *testing.T) {
	// `exec` must accept arbitrary flags after "--" to pass through to the container.
	assert.True(t, execCmd.FParseErrWhitelist.UnknownFlags, "exec should whitelist unknown flags")
}

func TestStackFlagIsPersistent(t *testing.T) {
	// The shared --stack/-s and --dry-run flags are inherited by subcommands.
	require.NotNil(t, emulatorCmd.PersistentFlags().Lookup("stack"))
	require.NotNil(t, emulatorCmd.PersistentFlags().Lookup("dry-run"))
}

func TestSubcommandSpecificFlags(t *testing.T) {
	// --ephemeral is only on `up`; --force is only on `reset`.
	require.NotNil(t, upCmd.Flags().Lookup("ephemeral"), "up should have --ephemeral")
	require.Nil(t, downCmd.Flags().Lookup("ephemeral"), "down should not have --ephemeral")
	require.NotNil(t, resetCmd.Flags().Lookup("force"), "reset should have --force")
	require.Nil(t, upCmd.Flags().Lookup("force"), "up should not have --force")
}

func TestVerbFlags_MapsEphemeralAndForce(t *testing.T) {
	restoreViperKey(t, "ephemeral")
	require.NoError(t, upCmd.Flags().Set("ephemeral", "true"))
	t.Cleanup(func() { _ = upCmd.Flags().Set("ephemeral", "false") })
	got := verbFlags(upCmd)
	assert.Equal(t, true, got["ephemeral"])
	_, hasForce := got["force"]
	assert.False(t, hasForce, "up has no --force flag, so it must not be in the map")

	restoreViperKey(t, "force")
	require.NoError(t, resetCmd.Flags().Set("force", "true"))
	t.Cleanup(func() { _ = resetCmd.Flags().Set("force", "false") })
	gotReset := verbFlags(resetCmd)
	assert.Equal(t, true, gotReset["force"])

	require.NoError(t, listCmd.Flags().Set(flagRuntime, "true"))
	t.Cleanup(func() { _ = listCmd.Flags().Set(flagRuntime, "false") })
	assert.Equal(t, true, verbFlags(listCmd)[flagRuntime])
}

func TestBuildConfigAndStacksInfo_FlagMapping(t *testing.T) {
	restoreViperKey(t, "stack")
	viper.GetViper().Set("stack", "")

	c := &cobra.Command{Use: "up"}
	c.Flags().String("stack", "", "")
	c.Flags().Bool("dry-run", false, "")
	c.Flags().String("base-path", "", "")
	require.NoError(t, c.Flags().Set("stack", "ue2-dev"))
	require.NoError(t, c.Flags().Set("dry-run", "true"))

	info := buildConfigAndStacksInfo(c)
	assert.Equal(t, "ue2-dev", info.Stack)
	assert.True(t, info.DryRun)
}

func TestBuildConfigAndStacksInfo_StackFromViperFallback(t *testing.T) {
	restoreViperKey(t, "stack")
	viper.GetViper().Set("stack", "from-env")

	// No --stack flag value set on the command, so the viper value is used.
	c := &cobra.Command{Use: "up"}
	c.Flags().String("stack", "", "")

	info := buildConfigAndStacksInfo(c)
	assert.Equal(t, "from-env", info.Stack)
	assert.False(t, info.DryRun)
}

func TestInitConfigAndStacksInfo_NoArgs(t *testing.T) {
	c := &cobra.Command{Use: "ps"}
	c.Flags().String("stack", "", "")

	info := initConfigAndStacksInfo(c, "ps", []string{})
	assert.Equal(t, cfg.EmulatorComponentType, info.ComponentType)
	assert.Equal(t, "ps", info.SubCommand)
	assert.Equal(t, []string{"emulator", "ps"}, info.CliArgs)
	assert.Empty(t, info.ComponentFromArg)
	assert.Empty(t, info.AdditionalArgsAndFlags)
}

func TestInitConfigAndStacksInfo_ComponentOnly(t *testing.T) {
	c := &cobra.Command{Use: "up"}
	c.Flags().String("stack", "", "")

	info := initConfigAndStacksInfo(c, "up", []string{"aws"})
	assert.Equal(t, "aws", info.ComponentFromArg)
	assert.Equal(t, "up", info.SubCommand)
	assert.Empty(t, info.AdditionalArgsAndFlags)
}

func TestInitConfigAndStacksInfo_ComponentThenDashCommand(t *testing.T) {
	// Simulate `emulator exec aws -- ls -la`: the component precedes "--" and the
	// pass-through command follows it.
	c := &cobra.Command{Use: "exec"}
	c.Flags().String("stack", "", "")
	require.NoError(t, c.ParseFlags([]string{"aws", "--", "ls", "-la"}))

	info := initConfigAndStacksInfo(c, "exec", c.Flags().Args())
	assert.Equal(t, "aws", info.ComponentFromArg)
	assert.Equal(t, []string{"ls", "-la"}, info.AdditionalArgsAndFlags)
}

func TestRunVerbParsesOnlyArgsBeforeDash(t *testing.T) {
	// Cobra retains pass-through arguments in Args(), including flags for the
	// command inside the emulator. The emulator parsers must never see those.
	c := &cobra.Command{Use: "exec"}
	c.Flags().String("stack", "", "")
	require.NoError(t, c.ParseFlags([]string{"aws", "--", "kubectl", "-n", "demo"}))

	positional, separated := flags.SplitArgsAtDash(c, c.Flags().Args())

	assert.Equal(t, []string{"aws"}, positional)
	assert.Equal(t, []string{"kubectl", "-n", "demo"}, separated)
}

func TestRequiresStack(t *testing.T) {
	for _, subcommand := range []string{"up", "down", "reset", "logs", "exec"} {
		assert.True(t, requiresStack(subcommand), "%s should require a component stack", subcommand)
	}
	assert.False(t, requiresStack("list"))
	assert.False(t, requiresStack("ps"))
}

func TestVerbFlagsOmitsRuntimeWhenDisabled(t *testing.T) {
	require.NoError(t, listCmd.Flags().Set(flagRuntime, "false"))
	assert.NotContains(t, verbFlags(listCmd), flagRuntime)
}

func TestRunVerbDispatchesInspectionCommand(t *testing.T) {
	provider := &capturingEmulatorProvider{}
	original, hadOriginal := component.GetProvider(cfg.EmulatorComponentType)
	require.NoError(t, component.Register(provider))
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, component.Register(original))
		}
	})

	restoreViperKey(t, "stack")
	_ = listCmd.Flags().Set(flagRuntime, "false")
	t.Cleanup(func() { _ = listCmd.Flags().Set(flagRuntime, "false") })

	require.NoError(t, runVerb(listCmd, "list", nil))
	require.NotNil(t, provider.execution)
	assert.Equal(t, cfg.EmulatorComponentType, provider.execution.ComponentType)
	assert.Equal(t, "list", provider.execution.SubCommand)
	assert.Empty(t, provider.execution.Component)
	assert.NotContains(t, provider.execution.Flags, flagRuntime)
}

func TestInitConfigAndStacksInfo_ExtraPositionalArgs(t *testing.T) {
	// Positional args beyond the component are carried as additional args.
	c := &cobra.Command{Use: "up"}
	c.Flags().String("stack", "", "")
	require.NoError(t, c.ParseFlags([]string{"aws", "extra"}))

	info := initConfigAndStacksInfo(c, "up", c.Flags().Args())
	assert.Equal(t, "aws", info.ComponentFromArg)
	assert.Equal(t, []string{"extra"}, info.AdditionalArgsAndFlags)
}

func TestRunVerb_ErrorOnMissingProject(t *testing.T) {
	// Point config discovery at an empty temp dir so there is no resolvable
	// emulator component; runVerb must surface an error rather than panicking.
	restoreViperKey(t, "base-path")
	restoreViperKey(t, "stack")
	viper.GetViper().Set("base-path", t.TempDir())
	viper.GetViper().Set("stack", "")

	c := &cobra.Command{Use: "up"}
	c.Flags().String("stack", "", "")
	require.NoError(t, c.Flags().Set("stack", "nonexistent"))

	err := runVerb(c, "up", []string{"does-not-exist"})
	require.Error(t, err)
}
