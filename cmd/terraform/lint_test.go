package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
