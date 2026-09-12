package hooks_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	_ "github.com/cloudposse/atmos/pkg/workflow"
)

func TestTestHookFailurePolicy(t *testing.T) {
	for _, policy := range []string{"fail", "warn", "ignore"} {
		t.Run(policy, func(t *testing.T) {
			var hook hooks.Hook
			require.NoError(t, yaml.Unmarshal([]byte(`kind: step
type: test
events: [after.terraform.apply]
with:
  name: smoke
  steps:
    - {name: failure, type: shell, command: "echo failed-hook-check; exit 1"}
    - {name: following, type: shell, command: "echo hidden-hook-success"}
`), &hook))
			hook.OnFailure = policy
			kind, ok := hooks.GetKind("step")
			require.True(t, ok)
			var output bytes.Buffer
			_, err := kind.Engine.Run(&hooks.ExecContext{Hook: &hook, Kind: kind, Event: hooks.AfterTerraformApply, AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{}, Stdout: &output, Stderr: &output})
			if policy == "fail" {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Contains(t, output.String(), "failed-hook-check")
			assert.Contains(t, ansi.Strip(output.String()), "1 passed, 1 failed")
			assert.NotContains(t, output.String(), "hidden-hook-success")
		})
	}
}

func TestTestHookMatrixTemplates(t *testing.T) {
	var hook hooks.Hook
	require.NoError(t, yaml.Unmarshal([]byte(`kind: step
type: test
on_failure: fail
with:
  name: smoke
  steps:
   - name: matrix
     type: matrix
     matrix: {region: [east, west]}
     steps:
      - name: check
        type: shell
        command: 'test -n "{{ .matrix.region }}"'
`), &hook))
	kind, ok := hooks.GetKind("step")
	require.True(t, ok)
	var output bytes.Buffer
	_, err := kind.Engine.Run(&hooks.ExecContext{Hook: &hook, Kind: kind, Event: hooks.AfterTerraformApply, AtmosConfig: &schema.AtmosConfiguration{}, Info: &schema.ConfigAndStacksInfo{}, Stderr: &output})
	require.NoError(t, err)
	assert.Contains(t, ansi.Strip(output.String()), "2 passed, 0 failed")
}
