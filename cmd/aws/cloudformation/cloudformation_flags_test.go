package cloudformation

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
)

// registerRecordingProvider swaps in a provider that records the
// ExecutionContext it receives, restoring the original on cleanup.
func registerRecordingProvider(t *testing.T) *recordingProvider {
	t.Helper()

	original, hadOriginal := component.GetProvider(cfg.CloudFormationComponentType)
	fake := &recordingProvider{}
	require.NoError(t, component.Register(fake))
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, component.Register(original))
		}
	})
	return fake
}

func TestValidateFlagSelection(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
		wantErr error
		wantMsg string
	}{
		{
			name:    "no flags",
			command: newOperationCommand("apply", "apply", "Apply"),
		},
		{
			name:    "all and affected",
			command: configuredOperationCommand(t, "apply", map[string]string{"all": "true", "affected": "true"}),
			wantErr: errUtils.ErrAwsCloudFormationFlagsMutuallyExclusive,
			wantMsg: "--all and --affected are mutually exclusive",
		},
		{
			name:    "include-dependents without affected",
			command: configuredOperationCommand(t, "apply", map[string]string{"include-dependents": "true", "all": "true"}),
			wantErr: errUtils.ErrAwsCloudFormationIncludeDependentsRequiresAffected,
			wantMsg: "--include-dependents requires --affected",
		},
		{
			name:    "malformed label",
			command: configuredOperationCommand(t, "apply", map[string]string{"labels": "not-valid"}),
			wantErr: errUtils.ErrInvalidFlag,
			wantMsg: "invalid label",
		},
		{
			name:    "valid labels",
			command: configuredOperationCommand(t, "apply", map[string]string{"labels": "cost-center=platform"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFlagSelection(tt.command)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.NotContains(t, err.Error(), "positional", "a flag error must not be headed as a positional-argument error")
			assert.NotContains(t, err.Error(), "\n", "the message must stay on one line")
		})
	}
}

// The combination errors must carry the flag-combination headline, not the
// positional-argument one the parser adds around validator errors.
func TestOperationCommandRunE_FlagCombinationErrorHeadline(t *testing.T) {
	fake := registerRecordingProvider(t)

	tests := []struct {
		name  string
		use   string
		flags map[string]string
		want  error
	}{
		{"all with affected", "apply", map[string]string{"all": "true", "affected": "true"}, errUtils.ErrAwsCloudFormationFlagsMutuallyExclusive},
		{"include-dependents without affected", "apply", map[string]string{"include-dependents": "true", "all": "true"}, errUtils.ErrAwsCloudFormationIncludeDependentsRequiresAffected},
		{"follow with chart", "logs", map[string]string{"follow": "true", "chart": "true"}, errUtils.ErrAwsCloudFormationLogsFollowChartExclusive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := configuredOperationCommand(t, tt.use, tt.flags)

			err := cmd.RunE(cmd, nil)
			require.ErrorIs(t, err, tt.want)
			require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationInvalidFlagCombination)
			require.NotErrorIs(t, err, errUtils.ErrInvalidPositionalArgs)
			assert.True(t, strings.HasPrefix(err.Error(), "invalid flag combination"), err.Error())
		})
	}
	assert.Empty(t, fake.executed, "an invalid combination must never reach the provider")
}

// A positional-argument mistake keeps its own (positional) headline.
func TestOperationCommandRunE_PositionalErrorKeepsPositionalHeadline(t *testing.T) {
	registerRecordingProvider(t)

	cmd := newOperationCommand("apply", "apply", "Apply")
	err := cmd.RunE(cmd, nil)
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationComponentArgRequired)
	require.ErrorIs(t, err, errUtils.ErrInvalidPositionalArgs)
}

func TestOutputCommand_AcceptsOptionalKeyArgument(t *testing.T) {
	outputCmd := newOperationCommand("output", "output", "Show output")
	assert.Equal(t, "output [component] [key]", outputCmd.Use)

	t.Run("cobra arg validator allows component and key", func(t *testing.T) {
		require.NoError(t, outputCmd.Args(outputCmd, []string{"vpc"}))
		require.NoError(t, outputCmd.Args(outputCmd, []string{"vpc", "VpcId"}))
		require.Error(t, outputCmd.Args(outputCmd, []string{"vpc", "VpcId", "extra"}))
	})

	t.Run("operation validator", func(t *testing.T) {
		require.NoError(t, validateOperationArgs(outputCmd, []string{"vpc"}))
		require.NoError(t, validateOperationArgs(outputCmd, []string{"vpc", "VpcId"}))
		require.ErrorIs(t, validateOperationArgs(outputCmd, []string{"vpc", "VpcId", "extra"}), errUtils.ErrAwsCloudFormationComponentArgRequired)
		require.ErrorIs(t, validateOperationArgs(outputCmd, nil), errUtils.ErrAwsCloudFormationComponentArgRequired)
	})

	t.Run("a key cannot be combined with a bulk selection", func(t *testing.T) {
		bulk := configuredOperationCommand(t, "output", map[string]string{"all": "true"})
		require.ErrorIs(t, validateOperationArgs(bulk, []string{"VpcId"}), errUtils.ErrAwsCloudFormationComponentArgWithSelection)
	})
}

// Only output/outputs takes a key; every other verb stays single-argument.
func TestOtherVerbsRejectASecondPositionalArgument(t *testing.T) {
	for _, verb := range []string{"apply", "diff", "delete", "render", "validate", "fmt", "logs"} {
		t.Run(verb, func(t *testing.T) {
			cmd := newOperationCommand(verb, verb, verb)
			require.Error(t, cmd.Args(cmd, []string{"vpc", "extra"}))
			require.ErrorIs(t, validateOperationArgs(cmd, []string{"vpc", "extra"}), errUtils.ErrAwsCloudFormationComponentArgRequired)
			assert.NotContains(t, cmd.Use, "[key]")
		})
	}
}

func TestOutputCommand_PassesKeyToProvider(t *testing.T) {
	fake := registerRecordingProvider(t)

	cmd := newOperationCommand("output", "output", "Show output")
	require.NoError(t, cmd.RunE(cmd, []string{"vpc", "VpcId"}))
	require.NoError(t, cmd.RunE(cmd, []string{"vpc"}))

	require.Len(t, fake.executed, 2)
	assert.Equal(t, "vpc", fake.executed[0].Component)
	assert.Equal(t, "VpcId", fake.executed[0].Flags["key"])
	assert.Equal(t, "vpc", fake.executed[1].Component)
	assert.NotContains(t, fake.executed[1].Flags, "key", "no key argument means all outputs")
}

func TestOutputCommandHelpDescribesKeyArgument(t *testing.T) {
	cmd := newOperationCommand("output", "output", "Show output")
	assert.Contains(t, cmd.Example, "output vpc VpcId --stack")
	assert.Contains(t, cmd.Long, "only that value")
}

// --skip-hooks must be a registered flag on exactly the hook-firing verbs.
func TestSkipHooksFlagRegisteredOnHookFiringVerbs(t *testing.T) {
	firing := []struct{ use, subCommand string }{
		{"diff", "diff"},
		{"plan", "diff"},
		{"apply", "apply"},
		{"deploy", "apply"},
		{"delete", "delete"},
		{"detect", "drift-detect"},
		{"describe", "drift-describe"},
	}
	for _, v := range firing {
		t.Run("fires "+v.use, func(t *testing.T) {
			cmd := newOperationCommand(v.use, v.subCommand, v.use)
			flag := cmd.Flag("skip-hooks")
			require.NotNil(t, flag, "--skip-hooks must be registered")
			assert.Equal(t, "*", flag.NoOptDefVal, "bare --skip-hooks skips all hooks")
		})
	}

	for _, v := range []struct{ use, subCommand string }{
		{"render", "render"},
		{"validate", "validate"},
		{"output", "output"},
		{"fmt", "fmt"},
		{"logs", "logs"},
		{"tree", "tree"},
		{"watch", "watch"},
		{"create", "changeset-create"},
	} {
		t.Run("silent "+v.use, func(t *testing.T) {
			cmd := newOperationCommand(v.use, v.subCommand, v.use)
			assert.Nil(t, cmd.Flag("skip-hooks"), "%s fires no hooks, so it must not accept --skip-hooks", v.use)
		})
	}
}

// --skip-hooks has the terraform semantics: no value skips everything,
// =a,b skips the named hooks, absent skips nothing.
func TestSkipHooksFlagSemantics(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantRaw    string
		skips      map[string]bool
		wantAllRun bool
	}{
		{name: "absent", args: nil, wantRaw: "", skips: map[string]bool{"a": false, "b": false}},
		{name: "bare flag skips all", args: []string{"--skip-hooks"}, wantRaw: "*", skips: map[string]bool{"a": true, "b": true}},
		{name: "named list", args: []string{"--skip-hooks=a,c"}, wantRaw: "a,c", skips: map[string]bool{"a": true, "b": false, "c": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			cmd := newOperationCommand("apply", "apply", "Apply")
			require.NoError(t, cmd.ParseFlags(tt.args))

			assert.Equal(t, tt.wantRaw, hooks.ResolveSkipHooks(cmd))
			skip := hooks.NewSkipPredicate(hooks.ResolveSkipHooks(cmd))
			for name, want := range tt.skips {
				assert.Equal(t, want, skip(name), "hook %q", name)
			}
		})
	}
}

// The executor resolves --skip-hooks through viper (it has no cobra command),
// so the value must reach viper when the verb runs. Before the flag existed the
// command rejected it as unknown while the hook log still named "--skip-hooks".
func TestSkipHooksReachesViperWhenVerbRuns(t *testing.T) {
	for _, verb := range []struct{ use, subCommand string }{
		{"diff", "diff"},
		{"apply", "apply"},
		{"delete", "delete"},
		{"detect", "drift-detect"},
		{"describe", "drift-describe"},
	} {
		t.Run(verb.use, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			registerRecordingProvider(t)

			cmd := newOperationCommand(verb.use, verb.subCommand, verb.use)
			require.NoError(t, cmd.ParseFlags([]string{"--skip-hooks=notify"}))
			require.NoError(t, cmd.RunE(cmd, []string{"vpc"}))

			assert.Equal(t, "notify", hooks.ResolveSkipHooks(nil))
		})
	}

	t.Run("environment variable", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		t.Setenv("ATMOS_SKIP_HOOKS", "*")
		registerRecordingProvider(t)

		cmd := newOperationCommand("apply", "apply", "Apply")
		require.NoError(t, cmd.RunE(cmd, []string{"vpc"}))

		assert.Equal(t, "*", hooks.ResolveSkipHooks(nil))
	})

	t.Run("not set leaves hooks running", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		registerRecordingProvider(t)

		cmd := newOperationCommand("apply", "apply", "Apply")
		require.NoError(t, cmd.RunE(cmd, []string{"vpc"}))

		assert.Empty(t, hooks.ResolveSkipHooks(nil))
	})
}

// The real `plan` and `deploy` commands record the verb the user ran, because
// they dispatch as `diff` and `apply`. Verbs that dispatch under their own name,
// and grouped verbs such as `changeset create`, record nothing.
func TestOperationFlags_RecordsTopLevelAliasVerb(t *testing.T) {
	find := func(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
		t.Helper()
		for _, c := range parent.Commands() {
			if c.Name() == name {
				return c
			}
		}
		require.FailNowf(t, "command not found", "%s under %s", name, parent.Name())
		return nil
	}

	assert.Equal(t, "plan", operationFlags(find(t, CloudFormationCmd, "plan"), opDiff, nil)[invokedVerbFlag])
	assert.Equal(t, "deploy", operationFlags(find(t, CloudFormationCmd, "deploy"), subCommandApply, nil)[invokedVerbFlag])
	assert.NotContains(t, operationFlags(find(t, CloudFormationCmd, "diff"), opDiff, nil), invokedVerbFlag)
	assert.NotContains(t, operationFlags(find(t, CloudFormationCmd, "apply"), subCommandApply, nil), invokedVerbFlag)

	changeset := find(t, CloudFormationCmd, "changeset")
	assert.NotContains(t, operationFlags(find(t, changeset, "create"), "changeset-create", nil), invokedVerbFlag)
}
