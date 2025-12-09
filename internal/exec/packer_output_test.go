package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecutePackerOutput(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/packer"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workDir)
	t.Setenv("ATMOS_LOGS_LEVEL", "Warning")

	// Test successful case (original test)
	t.Run("successful execution", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "output",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}

		packerFlags := PackerFlags{}

		d, err := ExecutePackerOutput(&info, &packerFlags)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(d.(map[string]any)["builds"].([]any)))
	})

	// Test missing stack
	t.Run("missing stack", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "output",
		}

		packerFlags := PackerFlags{}

		_, err := ExecutePackerOutput(&info, &packerFlags)
		assert.Error(t, err)
	})

	// Test invalid component path
	t.Run("invalid component path", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "invalid/component",
			SubCommand:       "output",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}

		packerFlags := PackerFlags{}

		_, err := ExecutePackerOutput(&info, &packerFlags)
		assert.Error(t, err)
	})

	// Test custom manifest filename
	t.Run("custom manifest filename", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "output",
			ProcessTemplates: true,
			ProcessFunctions: true,
			ComponentVarsSection: map[string]any{
				"manifest_file_name": "manifest.json",
			},
		}

		packerFlags := PackerFlags{}

		d, err := ExecutePackerOutput(&info, &packerFlags)
		assert.NoError(t, err)
		assert.NotNil(t, d)
	})

	// Test missing manifest file
	t.Run("missing manifest file", func(t *testing.T) {
		// Create a temporary directory for this test
		tempDir := t.TempDir()
		// Create a minimal packer config without a manifest file
		configPath := filepath.Join(tempDir, "atmos.yaml")
		err := os.WriteFile(configPath, []byte(`components:
  terraform:
    base_path: ""
  helmfile:
    base_path: ""
  packer:
    base_path: ""
`), 0o644)
		require.NoError(t, err)

		t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)

		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "nonexistent/component",
			SubCommand:       "output",
		}

		packerFlags := PackerFlags{}

		_, err = ExecutePackerOutput(&info, &packerFlags)
		assert.Error(t, err)
	})

	// Test invalid stack configuration
	t.Run("invalid stack configuration", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "invalid-stack",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "output",
		}

		packerFlags := PackerFlags{}

		_, err := ExecutePackerOutput(&info, &packerFlags)
		assert.Error(t, err)
	})

	t.Run("custom output format", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "output",
		}

		packerFlags := PackerFlags{}

		_, err := ExecutePackerOutput(&info, &packerFlags)
		assert.NoError(t, err)
	})

	// Test with invalid query
	t.Run("invalid query", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "output",
		}

		packerFlags := PackerFlags{
			Query: ".invalid.path",
		}

		r, err := ExecutePackerOutput(&info, &packerFlags)
		assert.NoError(t, err)
		assert.Equal(t, nil, r)
	})

	// Test query processing (original test cases)
	t.Run("query processing", func(t *testing.T) {
		info := schema.ConfigAndStacksInfo{
			StackFromArg:     "",
			Stack:            "nonprod",
			ComponentType:    "packer",
			ComponentFromArg: "aws/bastion",
			SubCommand:       "output",
			ProcessTemplates: true,
			ProcessFunctions: true,
		}

		t.Run("split query", func(t *testing.T) {
			packerFlags := PackerFlags{
				Query: ".builds[0].artifact_id | split(\":\")[1]",
			}
			d, err := ExecutePackerOutput(&info, &packerFlags)
			assert.NoError(t, err)
			assert.Equal(t, "ami-0c2ca16b7fcac7529", d)
		})

		t.Run("simple query", func(t *testing.T) {
			packerFlags := PackerFlags{
				Query: ".builds[0].artifact_id",
			}
			d, err := ExecutePackerOutput(&info, &packerFlags)
			assert.NoError(t, err)
			assert.Equal(t, "us-east-2:ami-0c2ca16b7fcac7529", d)
		})

		t.Run("invalid query", func(t *testing.T) {
			packerFlags := PackerFlags{
				Query: "invalid.query[",
			}
			_, err := ExecutePackerOutput(&info, &packerFlags)
			assert.Error(t, err)
		})
	})
}
