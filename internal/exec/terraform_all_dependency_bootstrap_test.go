package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// TestExecuteTerraformAll_DependencyBootstrapPreflight reproduces the reported
// regression: on a fresh environment, `atmos terraform apply --all` failed during the
// describe-stacks preflight because a dependent component's `!terraform.state` reference to a
// not-yet-applied component (e.g. `kms`) was resolved strictly, before the scheduler ever got a
// chance to apply that dependency. The dependency graph itself is built from the static
// `dependencies`/`settings.depends_on` sections (see TestBuildTerraformDependencyGraph), never
// from resolved vars, so the preflight only needs to tolerate — not correctly resolve — this
// kind of reference. DryRun keeps the test hermetic: no Terraform binary is invoked, and no
// state file exists for either component, matching a genuinely fresh environment.
//
// The table also covers the non-recoverable branch (a missing `!secret`) so a change that
// blanket-degrades every YAML function error, instead of only the intentionally-lenient
// `!terraform.state` case, would fail this regression guard.
func TestExecuteTerraformAll_DependencyBootstrapPreflight(t *testing.T) {
	tests := []struct {
		name       string
		stackYAML  string
		components []string
		wantErr    error
	}{
		{
			name: "unprovisioned terraform.state dependency does not fail preflight",
			stackYAML: `
components:
  terraform:
    kms:
      vars: {}
    audit-trail:
      dependencies:
        components:
          - name: kms
      vars:
        kms_key_arn: !terraform.state kms dev key_arn
`,
			components: []string{"kms", "audit-trail"},
			wantErr:    nil,
		},
		{
			name: "missing secret is non-recoverable and still fails preflight",
			stackYAML: `
components:
  terraform:
    app:
      secrets:
        vars:
          API_KEY:
            store: missing-secrets-store
            required: true
      vars:
        api_key: !secret API_KEY
`,
			components: []string{"app"},
			wantErr:    secrets.ErrStoreNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
			for _, component := range tt.components {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", component), 0o755))
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`
base_path: "."
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - "**/*"
`), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(tt.stackYAML), 0o600))

			t.Setenv("ATMOS_BASE_PATH", "")
			t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
			os.Unsetenv("ATMOS_BASE_PATH")
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
			t.Chdir(root)

			err := ExecuteTerraformAll(&schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentType:    "terraform",
				SubCommand:       "apply",
				DryRun:           true,
				ProcessTemplates: true,
				ProcessFunctions: true,
			})

			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}
