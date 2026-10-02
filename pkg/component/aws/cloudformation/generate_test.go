package cloudformation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.AwsCloudFormation{AutoGenerateFiles: true}

func generationFixture(t *testing.T, root, stack string) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) {
	t.Helper()
	config := &schema.AtmosConfiguration{BasePath: root}
	config.Components.CloudFormation.BasePath = "components/cloudformation"
	config.Components.CloudFormation.AutoGenerateFiles = true
	info := &schema.ConfigAndStacksInfo{
		Stack: stack, ComponentFromArg: "instance", FinalComponent: "shared", BaseComponentPath: "shared",
		ComponentVarsSection: map[string]any{"value": stack},
		ComponentSection: map[string]any{
			"component": "shared", "atmos_component": "instance", "atmos_stack": stack,
			"stack_name": "generated-" + stack, "path": "template.yaml",
			"stack_policy": map[string]any{"file": "policy.json"},
			"generate": map[string]any{
				"template.yaml":    "Resources: {}\nOutputs:\n  Value:\n    Value: '{{ .vars.value }}'\n",
				"policy.json":      map[string]any{"Statement": []any{map[string]any{"Effect": "Allow", "Resource": "*"}}},
				"nested/value.txt": "{{ .atmos_stack }}/{{ .atmos_component }}",
			},
		},
	}
	return config, info
}

// TestGeneratedComponentIsolation exercises real file generation and local provisioning.
func TestGeneratedComponentIsolation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(root, "components", "cloudformation", "shared")
	require.NoError(t, os.MkdirAll(filepath.Join(source, "shared"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "shared", "template.yaml"), []byte("wrong nested template"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "keep.txt"), []byte("original"), 0o600))
	for _, stack := range []string{"dev", "sandbox"} {
		t.Run(stack, func(t *testing.T) {
			t.Parallel()
			for range 2 {
				config, info := generationFixture(t, root, stack)
				spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationRender)
				require.NoError(t, err)
				assert.Contains(t, spec.TemplateBody, "Value: '"+stack+"'")
				assert.Contains(t, spec.TemplateAbsPath, filepath.Join(".workdir", "aws", "cloudformation"))
				var policy map[string]any
				require.NoError(t, json.Unmarshal([]byte(spec.StackPolicyBody), &policy))
				assert.Equal(t, "Allow", policy["Statement"].([]any)[0].(map[string]any)["Effect"])
				content, err := os.ReadFile(filepath.Join(filepath.Dir(spec.TemplateAbsPath), "nested", "value.txt"))
				require.NoError(t, err)
				assert.Equal(t, stack+"/instance", string(content))
				_, err = os.Stat(filepath.Join(source, "template.yaml"))
				assert.True(t, os.IsNotExist(err))
			}
		})
	}
}

func TestEntirelyGeneratedComponent(t *testing.T) {
	root := t.TempDir()
	config, info := generationFixture(t, root, "dev")
	spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationApply)
	require.NoError(t, err)
	assert.Contains(t, spec.TemplateBody, "Value: 'dev'")
	_, err = os.Stat(filepath.Join(root, "components"))
	assert.True(t, os.IsNotExist(err), "must not create shared source directories")
}

func TestGenerationConfigurationAndFailures(t *testing.T) {
	cases := []struct {
		name   string
		change func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo)
		want   error
	}{
		{"explicit disable", func(c *schema.AtmosConfiguration, i *schema.ConfigAndStacksInfo) {
			i.ComponentSection["provision"] = map[string]any{"workdir": map[string]any{"enabled": false}}
		}, errUtils.ErrInvalidAwsCloudFormationSettings},
		{"requires explicit path", func(c *schema.AtmosConfiguration, i *schema.ConfigAndStacksInfo) {
			delete(i.ComponentSection, "path")
			i.ComponentSection["source"] = map[string]any{"uri": "https://example.invalid/template.yaml"}
		}, errUtils.ErrMissingAwsCloudFormationTemplate},
		{"bad template", func(c *schema.AtmosConfiguration, i *schema.ConfigAndStacksInfo) {
			i.ComponentSection["generate"].(map[string]any)["template.yaml"] = "{{ missingFunction }}"
		}, errUtils.ErrInvalidAwsCloudFormationSettings},
		{"write failure", func(c *schema.AtmosConfiguration, i *schema.ConfigAndStacksInfo) {
			i.ComponentSection["generate"].(map[string]any)[".atmos"] = "cannot overwrite metadata directory"
		}, errUtils.ErrInvalidAwsCloudFormationSettings},
		{"escaping filename", func(c *schema.AtmosConfiguration, i *schema.ConfigAndStacksInfo) {
			i.ComponentSection["generate"].(map[string]any)["../escape.yaml"] = "Resources: {}"
		}, errUtils.ErrInvalidAwsCloudFormationSettings},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config, info := generationFixture(t, t.TempDir(), "dev")
			tc.change(config, info)
			_, err := resolveSpecAndTemplate(t.Context(), config, info, OperationRender)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestGenerationSkipsDeployedOperationsAndDryRun(t *testing.T) {
	for _, op := range []Operation{OperationDelete, OperationOutput, OperationDriftDetect, OperationGetTemplate, OperationLogs, OperationWatch, OperationStackSetInstances} {
		t.Run(string(op), func(t *testing.T) {
			root := t.TempDir()
			config, info := generationFixture(t, root, "dev")
			_, err := resolveSpecAndTemplate(t.Context(), config, info, op)
			require.NoError(t, err)
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
	config, info := generationFixture(t, t.TempDir(), "dev")
	info.ComponentSection["settings"] = map[string]any{cfg.CloudFormationComponentType: map[string]any{"region": "us-east-2"}}
	info.DryRun = true
	require.NoError(t, validateDryRun(config, info, nil, OperationRender))
	entries, err := os.ReadDir(config.BasePath)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.NotContains(t, info.ComponentSection, "provision", "dry-run must not mutate resolved config")
}

func TestGeneratedPolicyForNamedExecution(t *testing.T) {
	config, info := generationFixture(t, t.TempDir(), "dev")
	info.ComponentSection["path"] = "reviewed-template-not-needed.yaml"
	delete(info.ComponentSection["generate"].(map[string]any), "template.yaml")
	spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationChangesetExecute)
	require.NoError(t, err)
	assert.Contains(t, spec.StackPolicyBody, "Allow")
	assert.Empty(t, spec.TemplateBody, "named changesets keep their reviewed template")
}

func TestGenerationRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	config, info := generationFixture(t, root, "dev")
	require.NoError(t, prepareGeneration(config, info))
	path, _, err := provisionAndResolveComponentPath(t.Context(), provisioner.OutputWriters{}, config, info, cfg.CloudFormationComponentType, filepath.Join(root, "missing"))
	require.NoError(t, err)
	outside := t.TempDir()
	canary := filepath.Join(outside, "value.txt")
	require.NoError(t, os.WriteFile(canary, []byte("unchanged"), 0o600))
	if err := os.Symlink(outside, filepath.Join(path, "nested")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	require.ErrorIs(t, generateComponentFiles(config, info, path), errUtils.ErrInvalidAwsCloudFormationSettings)
	data, err := os.ReadFile(canary)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", string(data))
}

func TestDeployedDryRunIgnoresGenerationConfiguration(t *testing.T) {
	config, info := generationFixture(t, t.TempDir(), "dev")
	info.ComponentSection["provision"] = map[string]any{"workdir": map[string]any{"enabled": false}}
	info.ComponentSection["template"] = map[string]any{"Resources": map[string]any{}}
	delete(info.ComponentSection, "path")
	for _, op := range []Operation{OperationDelete, OperationOutput, OperationDriftDetect} {
		require.NoError(t, validateDryRun(config, info, nil, op))
	}
	files, err := os.ReadDir(config.BasePath)
	require.NoError(t, err)
	assert.Empty(t, files)
}
