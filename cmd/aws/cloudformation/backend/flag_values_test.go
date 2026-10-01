package backend

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetCommandFlagStack(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("stack", "", "")

	assert.Equal(t, "", getCommandFlagStack(cmd))

	assert.NoError(t, cmd.Flags().Set("stack", "dev"))
	assert.Equal(t, "dev", getCommandFlagStack(cmd))
}

func TestGetCommandFlagStack_InheritedFlag(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	parent.PersistentFlags().String("stack", "", "")
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)

	assert.NoError(t, parent.PersistentFlags().Set("stack", "prod"))
	// Force cobra to compute inherited flags.
	child.InheritedFlags()

	assert.Equal(t, "prod", getCommandFlagStack(child))
}

func TestGetCommandFlagBool(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("force", false, "")

	value, provided := getCommandFlagBool(cmd, "force")
	assert.False(t, value)
	assert.False(t, provided)

	assert.NoError(t, cmd.Flags().Set("force", "true"))
	value, provided = getCommandFlagBool(cmd, "force")
	assert.True(t, value)
	assert.True(t, provided)
}

func TestGetCommandFlagBool_MissingFlag(t *testing.T) {
	cmd := &cobra.Command{}
	value, provided := getCommandFlagBool(cmd, "does-not-exist")
	assert.False(t, value)
	assert.False(t, provided)
}

func TestComponentArgCompletion_WithArgsAlreadyProvided(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("stack", "", "")

	completions, directive := componentArgCompletion(cmd, []string{"already-provided"}, "")
	assert.Nil(t, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentArgCompletion_InitCliConfigError(t *testing.T) {
	orig := backendInitCliConfig
	t.Cleanup(func() { backendInitCliConfig = orig })
	backendInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errors.New("boom")
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("stack", "", "")

	completions, directive := componentArgCompletion(cmd, nil, "")
	assert.Nil(t, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentArgCompletion_DescribeStacksError(t *testing.T) {
	origInit := backendInitCliConfig
	origDescribe := backendDescribeStacks
	t.Cleanup(func() {
		backendInitCliConfig = origInit
		backendDescribeStacks = origDescribe
	})
	backendInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	backendDescribeStacks = func(
		*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager,
	) (map[string]any, error) {
		return nil, errors.New("boom")
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("stack", "", "")

	completions, directive := componentArgCompletion(cmd, nil, "")
	assert.Nil(t, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentArgCompletion_Success(t *testing.T) {
	origInit := backendInitCliConfig
	origDescribe := backendDescribeStacks
	origList := backendListAllComponents
	t.Cleanup(func() {
		backendInitCliConfig = origInit
		backendDescribeStacks = origDescribe
		backendListAllComponents = origList
	})
	backendInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	backendDescribeStacks = func(
		*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager,
	) (map[string]any, error) {
		return map[string]any{}, nil
	}
	backendListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
		return []string{"vpc", "eks"}, nil
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("stack", "", "")

	completions, directive := componentArgCompletion(cmd, nil, "")
	assert.Equal(t, []string{"vpc", "eks"}, completions)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}
