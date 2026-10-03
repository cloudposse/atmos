package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestComponentDefaultIdentitySupersedesStackDefault checks that the more specific default wins the merge:
// a stack-level default and a component-level default must not both survive, because a non-interactive
// run could not choose between them.
func TestComponentDefaultIdentitySupersedesStackDefault(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`base_path: "."
components:
  aws/cloudformation:
    base_path: components/cloudformation
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["*"]
  name_template: "{{ .vars.stage }}"
auth:
  identities:
    dev:
      kind: aws/user
    sandbox:
      kind: aws/user
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "test.yaml"), []byte(`vars:
  stage: test
auth:
  identities:
    dev:
      default: true
components:
  aws/cloudformation:
    inherits-stack-default:
      stack_name: a
      template:
        Resources: {}
    overrides-stack-default:
      stack_name: b
      template:
        Resources: {}
      auth:
        identities:
          sandbox:
            default: true
  terraform:
    tf-overrides-stack-default:
      auth:
        identities:
          sandbox:
            default: true
`), 0o600))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	for _, tc := range []struct {
		component   string
		wantDefault string
	}{
		{"inherits-stack-default", "dev"},
		{"overrides-stack-default", "sandbox"},
		{"tf-overrides-stack-default", "sandbox"},
	} {
		t.Run(tc.component, func(t *testing.T) {
			section, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
				AtmosConfig: &ac, Component: tc.component, Stack: "test",
			})
			require.NoError(t, err)
			identities := section[cfg.AuthSectionName].(map[string]any)["identities"].(map[string]any)
			var defaults []string
			for name, raw := range identities {
				if entry, ok := raw.(map[string]any); ok && entry["default"] == true {
					defaults = append(defaults, name)
				}
			}
			assert.Equal(t, []string{tc.wantDefault}, defaults)
		})
	}
}
