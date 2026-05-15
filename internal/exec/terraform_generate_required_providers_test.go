package exec

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newTestRequiredProvidersCmd builds a cobra.Command with the same flag set
// that `terraform generate required-providers` registers, so the flag-parser
// helper can be exercised in isolation without spinning up the full CLI tree.
//
// Mirrors cmd/terraform/generate/required_providers/required_providers.go plus
// the global flags (`process-templates`, `process-functions`, `skip`) that the
// helper queries.
func newTestRequiredProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "required-providers"}
	cmd.Flags().StringP("stack", "s", "", "")
	cmd.Flags().StringP("file", "f", "", "")
	cmd.Flags().Bool("process-templates", true, "")
	cmd.Flags().Bool("process-functions", true, "")
	cmd.Flags().StringSlice("skip", nil, "")
	return cmd
}

func TestParseRequiredProvidersFlags(t *testing.T) {
	t.Run("all defaults", func(t *testing.T) {
		cmd := newTestRequiredProvidersCmd()

		f, err := parseRequiredProvidersFlags(cmd)
		require.NoError(t, err)
		assert.Equal(t, "", f.stack)
		assert.True(t, f.processTemplates, "process-templates default is true")
		assert.True(t, f.processYamlFunctions, "process-functions default is true")
		assert.Empty(t, f.skip)
		assert.Equal(t, "", f.file)
	})

	t.Run("all flags set propagate through", func(t *testing.T) {
		cmd := newTestRequiredProvidersCmd()
		cmd.SetArgs([]string{
			"--stack", "dev-us-east-1",
			"--file", "custom.tf.json",
			"--process-templates=false",
			"--process-functions=false",
			"--skip", "!terraform.state",
			"--skip", "!terraform.output",
		})
		require.NoError(t, cmd.ParseFlags(cmd.Flags().Args()))
		// SetArgs alone doesn't trigger parsing without Execute; parse explicitly.
		require.NoError(t, cmd.ParseFlags([]string{
			"--stack", "dev-us-east-1",
			"--file", "custom.tf.json",
			"--process-templates=false",
			"--process-functions=false",
			"--skip", "!terraform.state",
			"--skip", "!terraform.output",
		}))

		f, err := parseRequiredProvidersFlags(cmd)
		require.NoError(t, err)
		assert.Equal(t, "dev-us-east-1", f.stack)
		assert.Equal(t, "custom.tf.json", f.file)
		assert.False(t, f.processTemplates)
		assert.False(t, f.processYamlFunctions)
		assert.Equal(t, []string{"!terraform.state", "!terraform.output"}, f.skip)
	})

	t.Run("missing 'file' flag returns empty (file is optional)", func(t *testing.T) {
		// Build a cmd that doesn't even register `file`. The helper deliberately
		// ignores the lookup error on `file` because the flag is optional —
		// asserting it survives a missing registration is the regression guard.
		cmd := &cobra.Command{Use: "required-providers"}
		cmd.Flags().StringP("stack", "s", "", "")
		cmd.Flags().Bool("process-templates", true, "")
		cmd.Flags().Bool("process-functions", true, "")
		cmd.Flags().StringSlice("skip", nil, "")

		f, err := parseRequiredProvidersFlags(cmd)
		require.NoError(t, err, "missing optional 'file' flag must not surface as an error")
		assert.Equal(t, "", f.file)
	})

	t.Run("missing required flag (stack) surfaces error", func(t *testing.T) {
		// Drop `stack` entirely so flags.GetString("stack") returns an error.
		cmd := &cobra.Command{Use: "required-providers"}
		cmd.Flags().StringP("file", "f", "", "")
		cmd.Flags().Bool("process-templates", true, "")
		cmd.Flags().Bool("process-functions", true, "")
		cmd.Flags().StringSlice("skip", nil, "")

		_, err := parseRequiredProvidersFlags(cmd)
		require.Error(t, err, "missing 'stack' flag registration must propagate as a lookup error")
	})
}

func TestConfigureCustomFilePath(t *testing.T) {
	// genCtx baseline — the helper only mutates CustomFilename / WorkingDir,
	// so anything else is irrelevant for these table-driven cases.
	makeCtx := func() *generator.GeneratorContext {
		return &generator.GeneratorContext{WorkingDir: "/original/workdir"}
	}

	tests := []struct {
		name            string
		fileFromArg     string
		wantWorkingDir  string // empty means "unchanged from baseline".
		wantCustomFile  string
		wantWorkingKept bool // when true, asserts the working dir is NOT changed.
	}{
		{
			name:            "empty arg is a no-op",
			fileFromArg:     "",
			wantWorkingKept: true,
		},
		{
			name:            "basename only sets filename and keeps workdir",
			fileFromArg:     "custom.tf.json",
			wantCustomFile:  "custom.tf.json",
			wantWorkingKept: true,
		},
		{
			name:            "current-dir prefix (./file) keeps workdir",
			fileFromArg:     "./custom.tf.json",
			wantCustomFile:  "custom.tf.json",
			wantWorkingKept: true,
		},
		{
			name:           "absolute path splits into dir+basename",
			fileFromArg:    "/tmp/generated/custom.tf.json",
			wantWorkingDir: "/tmp/generated",
			wantCustomFile: "custom.tf.json",
		},
		{
			name:           "relative path with subdir splits into dir+basename",
			fileFromArg:    "subdir/custom.tf.json",
			wantWorkingDir: "subdir",
			wantCustomFile: "custom.tf.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := makeCtx()
			configureCustomFilePath(ctx, tt.fileFromArg)

			assert.Equal(t, tt.wantCustomFile, ctx.CustomFilename)
			if tt.wantWorkingKept {
				assert.Equal(t, "/original/workdir", ctx.WorkingDir,
					"workdir must not be mutated when fileFromArg has no directory component")
			} else {
				assert.Equal(t, tt.wantWorkingDir, ctx.WorkingDir)
			}
		})
	}
}

func TestGenerateRequiredProvidersFile_EarlyReturns(t *testing.T) {
	// Each sub-test exercises a path that returns nil WITHOUT invoking the
	// underlying generator.Generate (which would need a real component on disk).
	// Together they cover the "nothing to generate" guard and the DryRun guard.
	t.Run("no required_version and no required_providers is a no-op", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg:  "vpc",
			Stack:             "dev",
			RequiredVersion:   "",
			RequiredProviders: nil,
		}

		err := generateRequiredProvidersFile(atmosConfig, info, "")
		assert.NoError(t, err, "missing version+providers must short-circuit cleanly, not error")
	})

	t.Run("DryRun skips the generator", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "dev",
			DryRun:           true,
			RequiredVersion:  ">= 1.10.1",
			RequiredProviders: map[string]map[string]any{
				"aws": {"source": "hashicorp/aws", "version": "~> 5.0"},
			},
		}

		err := generateRequiredProvidersFile(atmosConfig, info, "")
		assert.NoError(t, err, "DryRun must return nil without invoking generator.Generate")
	})

	t.Run("DryRun honors custom file path without writing", func(t *testing.T) {
		// Belt-and-suspenders: even when fileFromArg includes a custom path,
		// DryRun must still return cleanly with no side effects.
		atmosConfig := &schema.AtmosConfiguration{}
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "dev",
			DryRun:           true,
			RequiredVersion:  ">= 1.10.1",
		}

		err := generateRequiredProvidersFile(atmosConfig, info, "/tmp/custom.tf.json")
		assert.NoError(t, err)
	})
}

func TestExecuteTerraformGenerateRequiredProvidersCmd_ArgValidation(t *testing.T) {
	// The command is gated by cobra.ExactArgs(1) when invoked through the
	// real command tree, but the helper itself defends against the no-args
	// case directly. This test pins that defensive check.
	cmd := newTestRequiredProvidersCmd()

	tests := []struct {
		name string
		args []string
	}{
		{name: "zero args", args: []string{}},
		{name: "two args", args: []string{"vpc", "extra"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExecuteTerraformGenerateRequiredProvidersCmd(cmd, tt.args)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrInvalidComponentArgument),
				"%d args must surface ErrInvalidComponentArgument, got: %v", len(tt.args), err)
		})
	}
}
