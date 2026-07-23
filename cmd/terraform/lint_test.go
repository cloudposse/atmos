package terraform

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/scanners/tflint"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformLintCommandSetup(t *testing.T) {
	require.NotNil(t, lintParser)
	assert.True(t, lintParser.Registry().Has("affected"))
	assert.True(t, lintParser.Registry().Has("all"))
	assert.True(t, lintParser.Registry().Has(outputFormatFlagName))
	assert.NotNil(t, lintCmd.Flags().Lookup("affected"))
	assert.NotNil(t, lintCmd.Flags().Lookup("all"))
	assert.NotNil(t, lintCmd.Flags().Lookup(outputFormatFlagName))

	v := viper.New()
	require.NoError(t, lintParser.BindToViper(v))
	v.Set("affected", true)
	assert.True(t, v.GetBool("affected"))
	assert.Equal(t, outputFormatMarkdown, v.GetString(outputFormatFlagName))
}

func TestTerraformLintRuntimeCarriesOutputFormat(t *testing.T) {
	runtime := terraformLintRuntime("warn", outputFormatRich)
	assert.Equal(t, tflint.OutputFormatRich, runtime.OutputFormat)
}

func TestCheckTerraformLintFlags(t *testing.T) {
	tests := []struct {
		name    string
		info    *schema.ConfigAndStacksInfo
		wantErr bool
	}{
		{"component only", &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}, false},
		{"all only", &schema.ConfigAndStacksInfo{All: true}, false},
		{"affected only", &schema.ConfigAndStacksInfo{Affected: true}, false},
		{"component with all", &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", All: true}, true},
		{"component with affected", &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Affected: true}, true},
		{"all with affected", &schema.ConfigAndStacksInfo{All: true, Affected: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkTerraformLintFlags(tt.info)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTerraformLintAffectedArgsCopiesFlagsAndExecutionInfo(t *testing.T) {
	cmd := &cobra.Command{Use: "lint"}
	cmd.Flags().String("repo-path", "", "")
	cmd.Flags().String("ref", "", "")
	cmd.Flags().String("sha", "", "")
	cmd.Flags().String("ssh-key", "", "")
	cmd.Flags().String("ssh-key-password", "", "")
	cmd.Flags().Bool("clone-target-ref", false, "")
	cmd.Flags().Bool("include-dependents", false, "")
	require.NoError(t, cmd.Flags().Set("repo-path", "/repo"))
	require.NoError(t, cmd.Flags().Set("ref", "main"))
	require.NoError(t, cmd.Flags().Set("sha", "abc123"))
	require.NoError(t, cmd.Flags().Set("clone-target-ref", "true"))
	require.NoError(t, cmd.Flags().Set("include-dependents", "true"))

	info := &schema.ConfigAndStacksInfo{Stack: "plat-dev", ProcessTemplates: true, ProcessFunctions: true, Skip: []string{"terraform"}, AuthDisabled: true}
	args := terraformLintAffectedArgs(cmd, info, "warn")
	assert.Equal(t, "/repo", args.RepoPath)
	assert.Equal(t, "main", args.Ref)
	assert.Equal(t, "abc123", args.SHA)
	assert.True(t, args.CloneTargetRef)
	assert.Equal(t, info.Stack, args.Stack)
	assert.Equal(t, info.Skip, args.Skip)
	assert.True(t, args.AuthDisabled)
	assert.Equal(t, "warn", args.ErrorMode)
}

func TestRunTerraformLintDispatchesDirectAndAffectedModes(t *testing.T) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Skip("gomonkey binary patching is not supported on macOS ARM64")
	}

	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{Use: "lint"}
		cmd.Flags().String("repo-path", "", "")
		cmd.Flags().String("ref", "", "")
		cmd.Flags().String("sha", "", "")
		cmd.Flags().String("ssh-key", "", "")
		cmd.Flags().String("ssh-key-password", "", "")
		cmd.Flags().Bool("clone-target-ref", false, "")
		cmd.Flags().Bool("include-dependents", false, "")
		return cmd
	}

	t.Run("direct", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(internal.ValidateAtmosConfig, func(...internal.ValidateOption) error { return nil })
		patches.ApplyFunc(bindTerraformLintFlags, func(*cobra.Command, *viper.Viper) error { return nil })
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "dev"}
		patches.ApplyFunc(resolveTerraformLintInfo, func(*viper.Viper, []string) (*schema.ConfigAndStacksInfo, error) { return info, nil })
		called := false
		original := executeTerraformLint
		t.Cleanup(func() { executeTerraformLint = original })
		executeTerraformLint = func(_ context.Context, _ *tflint.Runtime, got *schema.ConfigAndStacksInfo, affected *tflint.AffectedOptions, _ int) error {
			called = true
			assert.Same(t, info, got)
			assert.Nil(t, affected)
			return nil
		}

		require.NoError(t, runTerraformLint(newCommand(), []string{"vpc"}))
		assert.True(t, called)
	})

	t.Run("affected", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(internal.ValidateAtmosConfig, func(...internal.ValidateOption) error { return nil })
		patches.ApplyFunc(bindTerraformLintFlags, func(*cobra.Command, *viper.Viper) error { return nil })
		info := &schema.ConfigAndStacksInfo{Affected: true, Stack: "dev", ProcessTemplates: true}
		patches.ApplyFunc(resolveTerraformLintInfo, func(*viper.Viper, []string) (*schema.ConfigAndStacksInfo, error) { return info, nil })
		called := false
		original := executeTerraformLint
		t.Cleanup(func() { executeTerraformLint = original })
		executeTerraformLint = func(_ context.Context, _ *tflint.Runtime, got *schema.ConfigAndStacksInfo, args *tflint.AffectedOptions, _ int) error {
			called = true
			assert.Same(t, info, got)
			require.NotNil(t, args)
			assert.Equal(t, "dev", args.Stack)
			assert.True(t, args.ProcessTemplates)
			return nil
		}

		require.NoError(t, runTerraformLint(newCommand(), nil))
		assert.True(t, called)
	})
}

func TestRunTerraformLintReturnsPreparationErrors(t *testing.T) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Skip("gomonkey binary patching is not supported on macOS ARM64")
	}

	t.Run("validation", func(t *testing.T) {
		want := errors.New("invalid config")
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(internal.ValidateAtmosConfig, func(...internal.ValidateOption) error { return want })
		require.ErrorIs(t, runTerraformLint(&cobra.Command{}, nil), want)
	})

	t.Run("information", func(t *testing.T) {
		want := errors.New("invalid flags")
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(internal.ValidateAtmosConfig, func(...internal.ValidateOption) error { return nil })
		patches.ApplyFunc(bindTerraformLintFlags, func(*cobra.Command, *viper.Viper) error { return nil })
		patches.ApplyFunc(resolveTerraformLintInfo, func(*viper.Viper, []string) (*schema.ConfigAndStacksInfo, error) { return nil, want })
		require.ErrorIs(t, runTerraformLint(&cobra.Command{}, nil), want)
	})
}

// TestResolveMaxFindings verifies --max-findings precedence (explicit flag/env value,
// including 0, wins over defaultMaxFindings) and the translation of the CLI's "0 =
// unlimited" convention into sarifUnlimitedFindings, the sentinel
// pkg/scanners/sarif's RenderMarkdownOptions recognizes.
func TestResolveMaxFindings(t *testing.T) {
	tests := []struct {
		name      string
		flagSet   bool // simulate cmd.Flags().Changed("max-findings")
		flagValue int  // value Viper reports for the flag
		want      int
	}{
		{
			name:      "user passes --max-findings 0 (unlimited, translated to sarif's sentinel)",
			flagSet:   true,
			flagValue: 0,
			want:      sarifUnlimitedFindings,
		},
		{
			name:      "user passes --max-findings 25 (explicit positive)",
			flagSet:   true,
			flagValue: 25,
			want:      25,
		},
		{
			name:      "env var sets 0 (no flag, viper picked up 0 via env binding)",
			flagSet:   false,
			flagValue: 0,
			want:      sarifUnlimitedFindings,
		},
		{
			name:      "env var sets 1000 (no flag, viper returns non-sentinel)",
			flagSet:   false,
			flagValue: 1000,
			want:      1000,
		},
		{
			name:      "no flag, no env — defaultMaxFindings",
			flagSet:   false,
			flagValue: maxFindingsUnset,
			want:      defaultMaxFindings,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newMaxFindingsTestCmd(t, tt.flagSet, tt.flagValue)
			assert.Equal(t, tt.want, resolveMaxFindings(cmd, tt.flagValue))
		})
	}

	t.Run("nil cmd does not panic and falls back to flag value when not sentinel", func(t *testing.T) {
		assert.Equal(t, 42, resolveMaxFindings(nil, 42))
	})

	t.Run("nil cmd with sentinel falls back to default", func(t *testing.T) {
		assert.Equal(t, defaultMaxFindings, resolveMaxFindings(nil, maxFindingsUnset))
	})
}

// newMaxFindingsTestCmd builds a tiny cobra command that registers a single
// --max-findings int flag with the same default sentinel as production, and
// optionally marks it as user-set by parsing an args slice.
func newMaxFindingsTestCmd(t *testing.T, flagSet bool, flagValue int) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test", Run: func(*cobra.Command, []string) {}}
	cmd.Flags().Int(maxFindingsFlagName, maxFindingsUnset, "")
	if flagSet {
		require.NoError(t, cmd.Flags().Set(maxFindingsFlagName, strconv.Itoa(flagValue)))
	}
	return cmd
}
