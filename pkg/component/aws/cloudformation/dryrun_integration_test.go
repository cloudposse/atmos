package cloudformation

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise actual stack loading: mocking ProcessStacks hid the dry-run side effect.
func TestDryRunDoesNotEvaluateStackFunctions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	marker := filepath.Join(dir, "executed")
	t.Setenv("_ATMOS_TEST_WRITE_MARKER", marker)
	binary, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`base_path: "."
components:
  aws/cloudformation:
    base_path: components/cloudformation
stacks:
  base_path: stacks
  included_paths: ["*"]
  name_template: "{{ .vars.stage }}"
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "dev.yaml"), []byte(fmt.Sprintf(`vars:
  stage: dev
components:
  aws/cloudformation:
    demo:
      stack_name: dry-demo
      template: !exec '"%s"'
      vars:
        marker: !exec '"%s"'
`, filepath.ToSlash(binary), filepath.ToSlash(binary))), 0o600))
	for _, all := range []bool{false, true} {
		for _, operation := range []Operation{OperationApply, OperationRender, OperationDelete} {
			info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentType: cfg.CloudFormationComponentType, DryRun: true, All: all}
			if !all {
				info.ComponentFromArg = "demo"
			}
			require.NoError(t, Execute(&component.ExecutionContext{ConfigAndStacksInfo: info}, operation))
			_, err := os.Stat(marker)
			assert.True(t, os.IsNotExist(err), "dry-run evaluated !exec")
		}
	}
}
