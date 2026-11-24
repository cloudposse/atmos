package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestStarlarkAffectedPolicyGate(t *testing.T) {
	basePath, base := affectedPolicyRepo(t, map[string]string{"stacks/prod.yaml": affectedTestManifest})
	headPath, head := affectedPolicyRepo(t, map[string]string{"README.md": "removed stack"})
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	document := `apiVersion: atmos/v1
kind: Policy
metadata: {name: starlark-affected}
spec:
  policies:
    protect:
      on: atmos.affected
      engine: starlark
      action: deny
      rule: |
        return any([i["action"] == "remove" for i in ctx.input["instances"]])
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "guard.yaml"), []byte(document), 0o600))
	err := enforceAffectedPolicies(affectedPolicyTestConfig(headPath), head, base, headPath, basePath)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.NoError(t, enforceAffectedPolicies(affectedPolicyTestConfig(headPath), head, head, headPath, headPath))
}
