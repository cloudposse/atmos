package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestScriptComponentExecutionResolution(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("STARLARK_COMPONENT_INPUT", "resolved-at-execution")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks", "deploy"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`base_path: .
stacks:
  base_path: stacks
  included_paths: ["deploy/**/*"]
  name_template: "{{.vars.stage}}"
components:
  script-resolver-test:
    base_path: apps
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(`vars:
  stage: dev
components:
  script-resolver-test:
    base:
      metadata: {type: abstract}
      vars: {replicas: 2}
    api:
      metadata:
        component: shared
        inherits: [base]
      env:
        INPUT: !env STARLARK_COMPONENT_INPUT
`), 0o600))
	resolver := ScriptComponentInfoResolver(&schema.AtmosConfiguration{BasePath: dir}, nil)
	result, err := resolver(context.Background(), "api", "dev", "script-resolver-test")
	require.NoError(t, err)
	assert.Equal(t, 2, result.ComponentVarsSection["replicas"])
	assert.Equal(t, "resolved-at-execution", result.ComponentEnvSection["INPUT"])
	assert.False(t, result.SecretsMaskOnly)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolver(ctx, "api", "dev", "script-resolver-test")
	require.ErrorIs(t, err, context.Canceled)
}
