package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStarlarkTerraformPolicyRegistration(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", directory)
	document := strings.Replace(testTerraformPolicyManifest,
		`rule: "has(input.resource_changes) && input.resource_changes.exists(r, 'delete' in r.change.actions)"`,
		"engine: starlark\n      rule: return True", 1)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "guard.yaml"), []byte(document), 0o600))
	gate, err := loadTerraformPlanPolicy(&schema.ConfigAndStacksInfo{SubCommand: "plan"})
	require.NoError(t, err)
	require.NotNil(t, gate)
	decision, err := gate.Engine.Evaluate(t.Context(), &policy.Request{
		Kind: policy.TerraformPlan, Input: json.RawMessage(`{"format_version":"1.2","planned_values":{}}`),
	}, gate.Rules)
	require.NoError(t, err)
	require.ErrorIs(t, policy.EnforceDecision(decision), errUtils.ErrPolicyDenied)
}

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
