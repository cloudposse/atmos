// Tests use: Go testing package with testify/assert and testify/require.
//go:build !integration
package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteTerraformGenerateBackends_WritesHCLBackendByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	useFakeFindStacksMap = true
	defer func() { useFakeFindStacksMap = false }()

	comp := "network/vpc"
	// Minimal stack having a single terraform component with backend config
	fakeStacksMap = map[string]any{
		filepath.Join("stacks", "tenant1", "ue2", "staging.yaml"): map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					comp: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"s3": map[string]any{
								"bucket": "my-bucket",
								"key":    "tfstate/tenant1/vpc.tfstate",
								"region": "us-east-2",
							},
						},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName:        map[string]any{"namespace": "eg", "environment": "staging", "tenant": "tenant1", "region": "us-east-2"},
					},
				},
			},
		},
	}

	// Prepare a terraform component folder
	atmosCfg := newMinimalAtmosConfigForTest(t, tmp)
	_ = prepareTerraformComponentDir(t, tmp, comp)

	err := ExecuteTerraformGenerateBackends(
		atmosCfg,
		"",          // fileTemplate -> write inside component path
		"",          // format empty -> default to hcl
		nil,         // stacks filter
		nil,         // components filter
	)
	require.NoError(t, err)

	// Assert backend.tf exists (HCL)
	out := filepath.Join(tmp, "components", "terraform", comp, "backend.tf")
	info, statErr := os.Stat(out)
	require.NoError(t, statErr)
	require.False(t, info.IsDir())
	// Validate contents contain terraform backend block
	data, rdErr := os.ReadFile(out)
	require.NoError(t, rdErr)
	assert.Contains(t, string(data), `terraform`)
	assert.Contains(t, string(data), `backend "s3"`)
	assert.Contains(t, string(data), `bucket`)
	assert.Contains(t, string(data), `key`)
}

func TestExecuteTerraformGenerateBackends_WritesJSONWhenRequested(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	useFakeFindStacksMap = true
	defer func() { useFakeFindStacksMap = false }()

	comp := "network/vpc"
	fakeStacksMap = map[string]any{
		"stacks/tenant1/ue2/staging.yaml": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					comp: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"s3": map[string]any{
								"bucket": "json-bucket",
								"key":    "tfstate/vpc.json",
								"region": "us-east-2",
							},
						},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName:        map[string]any{"namespace": "eg", "environment": "staging", "tenant": "tenant1", "region": "us-east-2"},
					},
				},
			},
		},
	}

	atmosCfg := newMinimalAtmosConfigForTest(t, tmp)
	_ = prepareTerraformComponentDir(t, tmp, comp)

	err := ExecuteTerraformGenerateBackends(atmosCfg, "", "json", nil, nil)
	require.NoError(t, err)

	out := filepath.Join(tmp, "components", "terraform", comp, "backend.tf.json")
	_, statErr := os.Stat(out)
	require.NoError(t, statErr)

	content, err := os.ReadFile(out)
	require.NoError(t, err)
	var obj map[string]any
	assert.NoError(t, json.Unmarshal(content, &obj))
}

func TestExecuteTerraformGenerateBackends_BackendConfigFormatWritesMap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	useFakeFindStacksMap = true
	defer func() { useFakeFindStacksMap = false }()

	comp := "app/web"
	fakeStacksMap = map[string]any{
		"stacks/tenant1/ue2/staging.yaml": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					comp: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"gcs": map[string]any{
								"bucket": "gcs-bucket",
								"prefix": "tfstate",
							},
						},
						cfg.BackendTypeSectionName: "gcs",
						cfg.VarsSectionName:        map[string]any{"namespace": "eg", "environment": "staging", "tenant": "tenant1", "region": "us-east-2"},
					},
				},
			},
		},
	}

	atmosCfg := newMinimalAtmosConfigForTest(t, tmp)
	_ = prepareTerraformComponentDir(t, tmp, comp)

	err := ExecuteTerraformGenerateBackends(atmosCfg, "", "backend-config", nil, nil)
	require.NoError(t, err)

	out := filepath.Join(tmp, "components", "terraform", comp, "backend.tf")
	_, statErr := os.Stat(out)
	require.NoError(t, statErr)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(data), "bucket")
	assert.Contains(t, string(data), "gcs")
}

func TestExecuteTerraformGenerateBackends_RespectsComponentsAndStacksFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	useFakeFindStacksMap = true
	defer func() { useFakeFindStacksMap = false }()

	compA := "network/vpc"
	compB := "app/web"
	fakeStacksMap = map[string]any{
		"orgs/cp/tenant1/staging/us-east-2.yaml": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					compA: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"s3": map[string]any{"bucket": "only-a", "key": "a.tfstate"},
						},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName:        map[string]any{"namespace": "eg", "environment": "staging", "tenant": "tenant1", "region": "us-east-2"},
					},
					compB: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"s3": map[string]any{"bucket": "only-b", "key": "b.tfstate"},
						},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName:        map[string]any{"namespace": "eg", "environment": "prod", "tenant": "tenant1", "region": "us-east-2"},
					},
				},
			},
		},
	}

	atmosCfg := newMinimalAtmosConfigForTest(t, tmp)
	_ = prepareTerraformComponentDir(t, tmp, compA)
	_ = prepareTerraformComponentDir(t, tmp, compB)

	// Filter to only compA
	err := ExecuteTerraformGenerateBackends(atmosCfg, "", "hcl", nil, []string{compA})
	require.NoError(t, err)

	// Comp A should exist, comp B should not
	a := filepath.Join(tmp, "components", "terraform", compA, "backend.tf")
	b := filepath.Join(tmp, "components", "terraform", compB, "backend.tf")
	_, errA := os.Stat(a)
	_, errB := os.Stat(b)
	assert.NoError(t, errA)
	assert.Error(t, errB)
}

func TestExecuteTerraformGenerateBackends_WithFileTemplateWritesToCustomLocation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := t.TempDir()
	useFakeFindStacksMap = true
	defer func() { useFakeFindStacksMap = false }()

	comp := "network/vpc"
	fakeStacksMap = map[string]any{
		"stacks/tenant1/ue2/staging.yaml": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					comp: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"s3": map[string]any{
								"bucket": "templ-bucket",
								"key":    "templ/xx.tfstate",
							},
						},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName: map[string]any{
							"namespace":   "eg",
							"environment": "staging",
							"tenant":      "tenant1",
							"region":      "us-east-2",
						},
					},
				},
			},
		},
	}

	atmosCfg := newMinimalAtmosConfigForTest(t, tmp)
	_ = prepareTerraformComponentDir(t, tmp, comp)

	// Use tokens in file template
	fileTemplate := filepath.Join(tmp, "out", "{tenant}", "{environment}", "{component}", "backend.tf")
	err := ExecuteTerraformGenerateBackends(atmosCfg, fileTemplate, "hcl", nil, nil)
	require.NoError(t, err)

	// Must write into custom path using context tokens
	out := filepath.Join(tmp, "out", "tenant1", "staging", strings.ReplaceAll(comp, "/", "-"), "backend.tf")
	_, statErr := os.Stat(out)
	require.NoError(t, statErr)
}

func TestExecuteTerraformGenerateBackendsCmd_InvalidFormatIsRejectedBeforeExecution(t *testing.T) {
	cmd := &cobra.Command{
		Use: "atmos",
	}
	// Set flags used by the command
	cmd.Flags().String("file-template", "")
	cmd.Flags().String("stacks", "")
	cmd.Flags().String("components", "")
	cmd.Flags().String("format", "")
	// Pass invalid format; we expect an error even though cfg.InitCliConfig would be called.
	_ = cmd.Flags().Set("format", "yaml")

	err := ExecuteTerraformGenerateBackendsCmd(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid '--format' argument")
}

// We shadow the real FindStacksMap with the fake one during tests via a small wrapper.
//go:linkname _findStacksMap exec.fakeFindStacksMap
func _findStacksMap(*schema.AtmosConfiguration, bool) (map[string]any, []string, error)

// Replace direct invocation site through a local wrapper used only in tests.
// NOTE: This relies on the compiler resolving the local symbol; if the project already
// provides indirection, this section will be ignored by build tags.
func init() {
	// switch to fake
	useFakeFindStacksMap = true
}