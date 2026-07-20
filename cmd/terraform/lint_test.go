package terraform

import (
	"errors"
	"runtime"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTerraformLintCommandSetup(t *testing.T) {
	require.NotNil(t, lintParser)
	assert.True(t, lintParser.Registry().Has("affected"))
	assert.True(t, lintParser.Registry().Has("all"))
	assert.NotNil(t, lintCmd.Flags().Lookup("affected"))
	assert.NotNil(t, lintCmd.Flags().Lookup("all"))

	v := viper.New()
	require.NoError(t, lintParser.BindToViper(v))
	v.Set("affected", true)
	assert.True(t, v.GetBool("affected"))
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
	args := terraformLintAffectedArgs(cmd, info)
	assert.Equal(t, "/repo", args.RepoPath)
	assert.Equal(t, "main", args.Ref)
	assert.Equal(t, "abc123", args.SHA)
	assert.True(t, args.CloneTargetRef)
	assert.True(t, args.IncludeDependents)
	assert.Equal(t, info.Stack, args.Stack)
	assert.Equal(t, info.Skip, args.Skip)
	assert.True(t, args.AuthDisabled)
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
		patches.ApplyFunc(e.ExecuteTerraformLint, func(got *schema.ConfigAndStacksInfo) error {
			called = true
			assert.Same(t, info, got)
			return nil
		})

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
		patches.ApplyFunc(e.ExecuteTerraformLintAffected, func(args *e.DescribeAffectedCmdArgs, got *schema.ConfigAndStacksInfo) error {
			called = true
			assert.Same(t, info, got)
			assert.Equal(t, "dev", args.Stack)
			assert.True(t, args.ProcessTemplates)
			return nil
		})

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
