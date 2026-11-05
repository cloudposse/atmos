package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateComponentSchemaDirectoryFlags(t *testing.T) {
	for _, invalid := range []string{"", "json", "opa"} {
		t.Run("invalid="+invalid, func(t *testing.T) {
			_ = NewTestKit(t)
			root := t.TempDir()
			files := map[string]string{
				"atmos.yaml": `base_path: .
stacks:
  base_path: stacks
  included_paths: [dev]
components:
  terraform:
    base_path: components
schemas:
  jsonschema: {base_path: missing-config-json}
  opa: {base_path: missing-config-opa}
`,
				"stacks/dev.yaml": `name: dev
components:
  terraform:
    app:
      vars: {message: good}
      settings:
        validation:
          json:
            schema_type: jsonschema
            schema_path: policy.json
          opa:
            schema_type: opa
            schema_path: policy.rego
`,
				"components/app/main.tf": "terraform {}\n",
				"good-json/policy.json":  `{"type":"object","properties":{"vars":{"properties":{"message":{"const":"good"}}}}}`,
				"good-opa/policy.rego":   "package atmos\nerrors[message] { input.vars.message != \"good\"; message = \"message must be good\" }\n",
				"bad-json/policy.json":   `{"type":"object","properties":{"vars":{"properties":{"message":{"const":"bad"}}}}}`,
				"bad-opa/policy.rego":    "package atmos\nerrors[message] { input.vars.message == \"good\"; message = \"expected denial\" }\n",
			}
			for name, content := range files {
				path := filepath.Join(root, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			}
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", root)
			t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
			t.Setenv("ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH", "missing-env-json")
			t.Setenv("ATMOS_SCHEMAS_OPA_BASE_PATH", "missing-env-opa")
			jsonDir, opaDir := "good-json", "good-opa"
			if invalid == "json" {
				jsonDir = "bad-json"
			}
			if invalid == "opa" {
				opaDir = "bad-opa"
			}
			// Exercise both flag forms, an absolute JSON base and a project-relative OPA base.
			setComponentCommandArgs(t, []string{"validate", "component", "app", "-s", "dev", "--schemas-jsonschema-dir=" + filepath.Join(root, jsonDir), "--schemas-opa-dir", opaDir})
			err := Execute()
			if invalid == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				if invalid == "opa" {
					require.Contains(t, err.Error(), "expected denial")
				} else {
					require.Contains(t, err.Error(), "validation")
				}
			}
		})
	}
}
