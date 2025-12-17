package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestParseCommonFlags_MissingStack tests that ParseCommonFlags returns an error when --stack is not provided.
func TestParseCommonFlags_MissingStack(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("identity", "", "", "Identity"),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	// Don't set --stack flag.
	opts, err := ParseCommonFlags(cmd, parser)

	require.Error(t, err)
	assert.Nil(t, opts)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestParseCommonFlags_WithStack tests that ParseCommonFlags works when --stack is provided.
func TestParseCommonFlags_WithStack(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("identity", "", "", "Identity"),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	// Set the flags via command line parsing.
	cmd.SetArgs([]string{"--stack", "dev-us-east-1"})
	err := cmd.ParseFlags([]string{"--stack", "dev-us-east-1"})
	require.NoError(t, err)

	opts, err := ParseCommonFlags(cmd, parser)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "dev-us-east-1", opts.Stack)
}

// TestParseCommonFlags_WithAllFlags tests parsing all common flags.
func TestParseCommonFlags_WithAllFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("identity", "", "", "Identity"),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "prod-us-west-2", "--identity", "my-identity", "--force"})
	require.NoError(t, err)

	opts, err := ParseCommonFlags(cmd, parser)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "prod-us-west-2", opts.Stack)
	assert.Equal(t, "my-identity", opts.Identity)
	assert.True(t, opts.Force)
}

// TestProvisionSource_NoMetadataSource tests that ProvisionSource returns nil when no source is configured.
func TestProvisionSource_NoMetadataSource(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory for the base path.
	tempDir := t.TempDir()

	opts := &ProvisionSourceOptions{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		Component:       "vpc",
		Stack:           "dev",
		ComponentConfig: map[string]any{}, // No metadata.source.
		AuthContext:     nil,
		Force:           false,
	}

	err := ProvisionSource(ctx, opts)
	assert.NoError(t, err, "ProvisionSource should return nil when no source is configured")
}

// TestProvisionSource_TargetExists tests that ProvisionSource skips when target exists and force=false.
func TestProvisionSource_TargetExists(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory with existing component.
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# existing"), 0o644)
	require.NoError(t, err)

	opts := &ProvisionSourceOptions{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		Component: "vpc",
		Stack:     "dev",
		ComponentConfig: map[string]any{
			"metadata": map[string]any{
				"source": map[string]any{
					"uri": "github.com/example/terraform-aws-vpc",
				},
			},
		},
		AuthContext: nil,
		Force:       false, // Not forcing.
	}

	err = ProvisionSource(ctx, opts)
	assert.NoError(t, err, "ProvisionSource should skip when target exists and force=false")

	// Verify existing file was not modified.
	content, err := os.ReadFile(filepath.Join(componentDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# existing", string(content))
}
