package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner"
)

func TestTerraformBackendInspection(t *testing.T) {
	for _, format := range []string{"table", "json", "yaml"} {
		for _, verb := range []string{"list", "describe"} {
			t.Run(verb+"/"+format, func(t *testing.T) {
				_ = NewTestKit(t)
				root := backendInspectionFixture(t)
				args := []string{"terraform", "backend", verb}
				if verb == "describe" {
					args = append(args, "a")
				}
				args = append(args, "-s", "dev", "--format", format, "--identity", "must-not-authenticate")
				stdout, err := executeBackendInspection(t, args)
				require.NoError(t, err)
				if format == "table" {
					require.Contains(t, stdout, "COMPONENT")
					require.Contains(t, stdout, "s3")
				} else {
					var result []provisioner.BackendConfiguration
					if verb == "describe" {
						var item provisioner.BackendConfiguration
						decodeBackendOutput(t, format, stdout, &item)
						result = append(result, item)
					} else {
						decodeBackendOutput(t, format, stdout, &result)
						require.Len(t, result, 3)
						require.Equal(t, "z", result[2].Component)
					}
					require.Equal(t, "a", result[0].Component)
					require.Equal(t, "dev", result[0].Stack)
					require.Equal(t, "shared", result[0].Backend["bucket"])
					require.Equal(t, "us-east-1", result[0].Backend["region"])
					require.Equal(t, "a.tfstate", result[0].Backend["key"])
					requireBackendExpressions(t, format, stdout, result[0].Backend)
					require.Equal(t, "{{ external.email }}", result[0].Backend["literal"])
					require.Equal(t, "{{ .vars.stage }}", result[0].Backend["template"])
				}
				require.NoFileExists(t, filepath.Join(root, "workspace-executed"))
				require.NoDirExists(t, filepath.Join(root, "components", "terraform", "shared", ".terraform"))
				require.NoFileExists(t, filepath.Join(root, "components", "terraform", "shared", "backend.tf.json"))
				require.NotContains(t, stdout, "__atmos_literal")
				require.NotContains(t, stdout, "!atmos-yaml-source:")
			})
		}
	}
}

func TestTerraformBackendInspectionSelections(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want error
	}{
		{"physical stack", []string{"list", "-s", "deploy/dev"}, nil},
		{"empty stack", []string{"list", "-s", "empty"}, nil},
		{"helmfile-only stack", []string{"list", "-s", "helm-only"}, nil},
		{"vars-only stack", []string{"list", "-s", "vars-only"}, nil},
		{"missing stack", []string{"list", "-s", "missing"}, errUtils.ErrStackNotFound},
		{"missing component", []string{"describe", "missing", "-s", "dev"}, errUtils.ErrInvalidComponent},
		{"other component type", []string{"describe", "helm", "-s", "dev"}, errUtils.ErrInvalidComponent},
		{"abstract component", []string{"describe", "base", "-s", "dev"}, errUtils.ErrInvalidComponent},
		{"without backend", []string{"describe", "none", "-s", "dev"}, errUtils.ErrBackendNotFound},
		{"missing selector", []string{"list"}, errUtils.ErrRequiredFlagNotProvided},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			backendInspectionFixture(t)
			args := append([]string{"terraform", "backend"}, tc.args...)
			args = append(args, "--format", "json")
			stdout, err := executeBackendInspection(t, args)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			} else {
				require.NoError(t, err)
			}
			if tc.name == "empty stack" || tc.name == "helmfile-only stack" || tc.name == "vars-only stack" {
				require.JSONEq(t, "[]", stdout)
			}
		})
	}
}

func TestTerraformBackendInspectionRejectsDynamicStackName(t *testing.T) {
	_ = NewTestKit(t)
	root := backendInspectionFixture(t)
	configPath := filepath.Join(root, "atmos.yaml")
	contents, err := os.ReadFile(configPath)
	require.NoError(t, err)
	updated := strings.Replace(string(contents), "{{ .vars.stage }}", "{{ env.Getenv `ATMOS_SHOULD_NOT_BE_READ` }}", 1)
	require.NoError(t, os.WriteFile(configPath, []byte(updated), 0o600))
	_, err = executeBackendInspection(t, []string{"terraform", "backend", "list", "-s", "dev", "--format", "json"})
	require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
	require.Contains(t, err.Error(), "without evaluating dynamic expressions")
}

func TestTerraformBackendInspectionConfigOverrides(t *testing.T) {
	_ = NewTestKit(t)
	root := backendInspectionFixture(t)
	override := filepath.Join(root, "override.yaml")
	config, err := os.ReadFile(filepath.Join(root, "atmos.yaml"))
	require.NoError(t, err)
	selected := strings.Replace(string(config), "included_paths: [deploy/**/*]", "included_paths: [deploy/empty]", 1)
	require.NoError(t, os.WriteFile(override, []byte(selected), 0o600))
	stdout, err := executeBackendInspection(t, []string{"--config", override, "--base-path", root, "terraform", "backend", "list", "-s", "empty", "--format", "json"})
	require.NoError(t, err)
	require.JSONEq(t, "[]", stdout)
}

func backendInspectionFixture(t *testing.T) string {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	clearIdentityEnvVars(t)
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv("ATMOS_INTERACTIVE", "false")
	for _, path := range []string{"stacks/catalog", "stacks/deploy", "components/terraform/shared"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, path), 0o755))
	}
	files := map[string]string{
		"atmos.yaml": `base_path: .
stacks:
  base_path: stacks
  included_paths: [deploy/**/*]
  name_template: '{{ .vars.stage }}'
components:
  terraform:
    base_path: components/terraform
templates:
  settings:
    enabled: true
`,
		"stacks/catalog/base.yaml": `locals:
  forbidden: !exec imported-command-that-must-not-run
components:
  terraform:
    base:
      metadata:
        type: abstract
      backend_type: s3
      backend:
        s3:
          bucket: shared
          region: us-east-1
          workspace_key_prefix: shared
          unresolved: !exec command-that-must-not-run
          include: !include https://must-not-fetch.invalid/config.yaml
          include_raw: !include.raw missing-file-must-not-read
          literal: !literal '{{ external.email }}'
          template: '{{ .vars.stage }}'
`,
		"stacks/deploy/dev.yaml": fmt.Sprintf(`import: [catalog/base]
locals:
  forbidden: !exec top-level-command-that-must-not-run
vars:
  stage: dev
components:
  terraform:
    a:
      metadata:
        component: shared
        inherits: [base]
        terraform_workspace_template: '{{ file.Write %q "executed" }}'
      backend:
        s3:
          key: a.tfstate
      auth:
        identities:
          forbidden:
            kind: unsupported/must-not-authenticate
            default: true
    z:
      metadata:
        component: shared
        inherits: [base]
    custom:
      backend_type: arbitrary
      backend:
        arbitrary:
          address: example
    none: {}
  helmfile:
    helm: {}
`, filepath.ToSlash(filepath.Join(root, "workspace-executed"))),
		"stacks/deploy/empty.yaml":            "name: empty\ncomponents:\n  terraform:\n    none: {}\n",
		"stacks/deploy/helm-only.yaml":        "components:\n  helmfile:\n    only:\n      vars:\n        stage: helm-only\n",
		"stacks/deploy/vars-only.yaml":        "vars:\n  stage: vars-only\n",
		"components/terraform/shared/main.tf": "terraform {}\n",
	}
	for path, contents := range files {
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(contents), 0o600))
	}
	return root
}

func executeBackendInspection(t *testing.T, args []string) (string, error) {
	t.Helper()
	setComponentCommandArgs(t, args)
	var err error
	stdout, _ := captureStdoutStderr(t, func() { err = Execute() })
	return stdout, err
}

func decodeBackendOutput(t *testing.T, format, output string, target any) {
	t.Helper()
	if format == "json" {
		require.NoError(t, json.Unmarshal([]byte(output), target), output)
	} else {
		require.NoError(t, yaml.Unmarshal([]byte(output), target), output)
	}
}

func requireBackendYAMLTag(t *testing.T, output, key, tag, argument string) {
	t.Helper()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(output), &doc))
	node := doc.Content[0]
	if node.Kind == yaml.SequenceNode {
		node = node.Content[0]
	}
	for _, name := range []string{"backend", key} {
		require.Equal(t, yaml.MappingNode, node.Kind)
		var value *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == name {
				value = node.Content[i+1]
				break
			}
		}
		require.NotNil(t, value, "missing field %s", name)
		node = value
	}
	require.Equal(t, tag, node.Tag)
	require.Equal(t, argument, node.Value)
}

func requireBackendExpressions(t *testing.T, format, stdout string, backend map[string]any) {
	t.Helper()
	for key, expression := range map[string]string{
		"unresolved":  "!exec command-that-must-not-run",
		"include":     "!include https://must-not-fetch.invalid/config.yaml",
		"include_raw": "!include.raw missing-file-must-not-read",
	} {
		tag, argument, _ := strings.Cut(expression, " ")
		if format == "yaml" {
			require.Equal(t, argument, backend[key])
			requireBackendYAMLTag(t, stdout, key, tag, argument)
		} else {
			require.Equal(t, expression, backend[key])
		}
	}
}
