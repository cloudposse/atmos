package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// deleteTypeFixture describes one component type's layout for target-resolution tests.
type deleteTypeFixture struct {
	componentType string
	configure     func(base string, config *schema.AtmosConfiguration)
	dir           func(base, name string) string
}

func deleteTypeFixtures() []deleteTypeFixture {
	return []deleteTypeFixture{
		{
			componentType: "terraform",
			configure: func(base string, config *schema.AtmosConfiguration) {
				config.Components.Terraform.BasePath = filepath.Join(base, "components", "terraform")
			},
			dir: func(base, name string) string { return filepath.Join(base, "components", "terraform", name) },
		},
		{
			componentType: "aws/cloudformation",
			configure: func(base string, config *schema.AtmosConfiguration) {
				config.Components.CloudFormation.BasePath = filepath.Join(base, "components", "cloudformation")
			},
			dir: func(base, name string) string { return filepath.Join(base, "components", "cloudformation", name) },
		},
	}
}

func writeComponentDir(t *testing.T, dir string, provisioned bool) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("# test"), 0o600))
	if provisioned {
		markProvisioned(t, dir)
	}
}

// TestDeleteSourceDirectory_TargetsRuntimeDirectory verifies delete acts on the directory the
// runtime provisions into (metadata.component), never on a directory named after the instance.
func TestDeleteSourceDirectory_TargetsRuntimeDirectory(t *testing.T) {
	for _, fx := range deleteTypeFixtures() {
		t.Run(fx.componentType, func(t *testing.T) {
			base := t.TempDir()
			config := &schema.AtmosConfiguration{BasePath: base}
			fx.configure(base, config)

			// The instance name collides with a hand-written directory owned by another component.
			handmade := fx.dir(base, "handmade")
			sourced := fx.dir(base, "src-nested")
			writeComponentDir(t, handmade, false)
			writeComponentDir(t, sourced, true)
			stubDescribeStacks(t, map[string]any{
				"x-handmade": map[string]any{"component": "handmade"},
				"handmade":   map[string]any{"component": "src-nested", "source": map[string]any{"uri": "https://example.invalid/x"}},
			})

			section := map[string]any{
				"source":          map[string]any{"uri": "https://example.invalid/x"},
				"atmos_component": "handmade",
				"component":       "src-nested",
				"metadata":        map[string]any{"component": "src-nested"},
			}
			require.NoError(t, deleteSourceDirectory(config, testDeleteRequest(fx.componentType, "handmade", section)))

			assert.NoDirExists(t, sourced, "the directory the runtime uses is deleted")
			assert.FileExists(t, filepath.Join(handmade, "main.tf"), "a same-named hand-written directory is untouched")
		})
	}
}

// TestDeleteSourceDirectory_Refusals verifies the safety guard.
func TestDeleteSourceDirectory_Refusals(t *testing.T) {
	cases := []struct {
		name        string
		provisioned bool
		siblings    map[string]any
	}{
		{name: "no provenance marker", siblings: map[string]any{}},
		{
			name:        "owned by component without source",
			provisioned: true,
			siblings:    map[string]any{"x-handmade": map[string]any{"component": "vpc"}},
		},
	}
	for _, fx := range deleteTypeFixtures() {
		for _, tt := range cases {
			t.Run(fx.componentType+"/"+tt.name, func(t *testing.T) {
				base := t.TempDir()
				config := &schema.AtmosConfiguration{BasePath: base}
				fx.configure(base, config)
				target := fx.dir(base, "vpc")
				writeComponentDir(t, target, tt.provisioned)
				stubDescribeStacks(t, tt.siblings)

				section := map[string]any{"source": map[string]any{"uri": "https://example.invalid/x"}, "atmos_component": "vpc", "component": "vpc"}
				err := deleteSourceDirectory(config, testDeleteRequest(fx.componentType, "vpc", section))

				require.ErrorIs(t, err, errUtils.ErrSourceDeleteRefused)
				assert.FileExists(t, filepath.Join(target, "main.tf"), "a refused directory is never deleted")
			})
		}
	}
}

// TestDeleteSourceDirectory_FailsClosedWhenStackUnreadable verifies delete refuses when ownership
// cannot be verified.
func TestDeleteSourceDirectory_FailsClosedWhenStackUnreadable(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = filepath.Join(base, "components", "terraform")
	target := filepath.Join(base, "components", "terraform", "vpc")
	writeComponentDir(t, target, true)

	stubDescribeStacks(t, nil)
	executeDescribeStacksFunc = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return nil, assert.AnError
	}

	section := map[string]any{"source": map[string]any{"uri": "https://example.invalid/x"}, "atmos_component": "vpc"}
	err := deleteSourceDirectory(config, testDeleteRequest("terraform", "vpc", section))

	require.ErrorIs(t, err, errUtils.ErrExecuteDescribeStacks)
	assert.FileExists(t, filepath.Join(target, "main.tf"))
}

// TestDeleteSourceDirectory_FailsClosedWhenConfigUnreadable verifies delete refuses when the stack
// configuration needed to verify ownership cannot be loaded.
func TestDeleteSourceDirectory_FailsClosedWhenConfigUnreadable(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePath: base}
	config.Components.Terraform.BasePath = filepath.Join(base, "components", "terraform")
	target := filepath.Join(base, "components", "terraform", "vpc")
	writeComponentDir(t, target, true)

	stubDescribeStacks(t, nil)
	initCliConfigForPrompt = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, assert.AnError
	}

	section := map[string]any{"source": map[string]any{"uri": "https://example.invalid/x"}, "atmos_component": "vpc"}
	err := deleteSourceDirectory(config, testDeleteRequest("terraform", "vpc", section))

	require.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
	assert.FileExists(t, filepath.Join(target, "main.tf"))
}
