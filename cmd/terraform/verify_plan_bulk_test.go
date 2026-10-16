package terraform

// verify_plan_bulk_test.go covers planfile verification for multi-component
// deploys (--affected / --all / --components). Those runs never reach the
// single-component verifyStoredPlanForDeploy call in RunE; each scheduler node
// runs terraformNodeHooks.BeforeWithWriters instead, so the per-node verify and
// required gates must be enforced there.
//
// Note on test isolation: cmd.NewTestKit(t) cannot be used here due to a
// circular import between cmd/terraform and the cmd package (see
// utils_hooks_test.go). These tests do not mutate RootCmd state.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// verifyPlanfileCall records one invocation of the stubbed verifyPlanfile seam.
type verifyPlanfileCall struct {
	storedPlanFile string
	mode           schema.PlanfileVerifyMode
}

// stubVerifyPlanfile replaces the verifyPlanfile seam for the duration of the
// test with fake and returns the recorded calls.
func stubVerifyPlanfile(t *testing.T, fake func(info *schema.ConfigAndStacksInfo, storedPlanFile string) error) *[]verifyPlanfileCall {
	t.Helper()

	calls := &[]verifyPlanfileCall{}
	original := verifyPlanfile
	t.Cleanup(func() { verifyPlanfile = original })
	verifyPlanfile = func(info *schema.ConfigAndStacksInfo, storedPlanFile string, mode schema.PlanfileVerifyMode) error {
		*calls = append(*calls, verifyPlanfileCall{storedPlanFile: storedPlanFile, mode: mode})
		return fake(info, storedPlanFile)
	}
	return calls
}

// writeStoredPlan places a stored planfile where verifyStoredPlanForDeploy looks
// for it (next to the canonical planfile) and returns its path.
func writeStoredPlan(t *testing.T, info *schema.ConfigAndStacksInfo) string {
	t.Helper()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	require.NoError(t, err)
	canonical := e.ConstructTerraformComponentPlanfilePath(&atmosConfig, info)
	stored := filepath.Join(filepath.Dir(canonical), planfile.StoredPlanPrefix+planfile.PlanFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(stored), 0o755))
	require.NoError(t, os.WriteFile(stored, []byte("stored plan"), 0o644))
	return stored
}

// freshPlanPath is the fresh planfile path the matching fake verification selects.
func freshPlanPath(storedPlanFile string) string {
	return filepath.Join(filepath.Dir(storedPlanFile), "fresh.planfile")
}

// fakeVerifyDrift simulates a stored plan that differs from the fresh plan.
func fakeVerifyDrift(_ *schema.ConfigAndStacksInfo, _ string) error {
	return fmt.Errorf("%w: fake drift", errUtils.ErrPlanVerificationFailed)
}

// fakeVerifyMatch simulates a stored plan that matches the fresh plan, mirroring
// finalizeVerification: info is pointed at the fresh plan for apply.
func fakeVerifyMatch(info *schema.ConfigAndStacksInfo, storedPlanFile string) error {
	info.PlanFile = freshPlanPath(storedPlanFile)
	info.UseTerraformPlan = true
	return nil
}

const storagePlanfilesYAML = "    planfiles:\n      priority:\n        - github\n"

func TestBeforeWithWriters_BulkDeployVerifiesStoredPlan(t *testing.T) {
	tests := []struct {
		name           string
		subCommand     string
		beforeEvent    hooks.HookEvent
		planfilesYAML  string
		verifyPlanMode schema.PlanfileVerifyMode
		storedPlan     bool
		fake           func(info *schema.ConfigAndStacksInfo, storedPlanFile string) error
		wantErr        error
		wantCalls      int
		wantFreshPlan  bool
	}{
		{
			name:           "deploy with drifted stored plan fails verification",
			subCommand:     "deploy",
			beforeEvent:    hooks.BeforeTerraformDeploy,
			planfilesYAML:  storagePlanfilesYAML,
			verifyPlanMode: schema.PlanfileVerifyFail,
			storedPlan:     true,
			fake:           fakeVerifyDrift,
			wantErr:        errUtils.ErrPlanVerificationFailed,
			wantCalls:      1,
		},
		{
			name:           "deploy without stored plan under required true fails",
			subCommand:     "deploy",
			beforeEvent:    hooks.BeforeTerraformDeploy,
			planfilesYAML:  storagePlanfilesYAML + "      verify: fail\n      required: true\n",
			verifyPlanMode: "",
			fake:           fakeVerifyMatch,
			wantErr:        errUtils.ErrStoredPlanfileMissing,
		},
		{
			name:           "deploy without stored plan under verify fail and required unset fails",
			subCommand:     "deploy",
			beforeEvent:    hooks.BeforeTerraformDeploy,
			planfilesYAML:  storagePlanfilesYAML + "      verify: fail\n",
			verifyPlanMode: "",
			fake:           fakeVerifyMatch,
			wantErr:        errUtils.ErrStoredPlanfileMissing,
		},
		{
			name:           "deploy without stored plan under --verify-plan fails",
			subCommand:     "deploy",
			beforeEvent:    hooks.BeforeTerraformDeploy,
			planfilesYAML:  storagePlanfilesYAML,
			verifyPlanMode: schema.PlanfileVerifyFail,
			fake:           fakeVerifyMatch,
			wantErr:        errUtils.ErrStoredPlanfileMissing,
		},
		{
			name:           "deploy with matching stored plan selects the fresh plan",
			subCommand:     "deploy",
			beforeEvent:    hooks.BeforeTerraformDeploy,
			planfilesYAML:  storagePlanfilesYAML,
			verifyPlanMode: schema.PlanfileVerifyFail,
			storedPlan:     true,
			fake:           fakeVerifyMatch,
			wantCalls:      1,
			wantFreshPlan:  true,
		},
		{
			name:           "deploy without planfile storage or verify settings does not verify",
			subCommand:     "deploy",
			beforeEvent:    hooks.BeforeTerraformDeploy,
			planfilesYAML:  "",
			verifyPlanMode: "",
			storedPlan:     true,
			fake:           fakeVerifyDrift,
		},
		{
			name:           "apply with stored plan does not verify",
			subCommand:     "apply",
			beforeEvent:    hooks.BeforeTerraformApply,
			planfilesYAML:  storagePlanfilesYAML,
			verifyPlanMode: schema.PlanfileVerifyFail,
			storedPlan:     true,
			fake:           fakeVerifyDrift,
		},
		{
			name:           "plan with stored plan does not verify",
			subCommand:     "plan",
			beforeEvent:    hooks.BeforeTerraformPlan,
			planfilesYAML:  storagePlanfilesYAML,
			verifyPlanMode: schema.PlanfileVerifyFail,
			storedPlan:     true,
			fake:           fakeVerifyDrift,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withoutCIDetection(t)
			info := writeTempAtmosProjectWithPlanfiles(t, tt.planfilesYAML)
			info.VerifyPlanMode = tt.verifyPlanMode
			info.ComponentType = cfg.TerraformComponentType

			var stored string
			if tt.storedPlan {
				stored = writeStoredPlan(t, info)
			}
			calls := stubVerifyPlanfile(t, tt.fake)

			nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), subCommand: tt.subCommand, beforeEvent: tt.beforeEvent}
			err := nodeHooks.BeforeWithWriters(context.Background(), info, schema.ComponentNodeHookWriters{})

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			require.Len(t, *calls, tt.wantCalls, "unexpected number of stored-vs-fresh plan comparisons")
			if tt.wantCalls > 0 {
				assert.Equal(t, stored, (*calls)[0].storedPlanFile, "verification must compare against the downloaded stored plan")
				assert.Equal(t, schema.PlanfileVerifyFail, (*calls)[0].mode)
			}

			if tt.wantFreshPlan {
				assert.Equal(t, freshPlanPath(stored), info.PlanFile, "apply must use the verified fresh plan")
				assert.True(t, info.UseTerraformPlan, "apply must be told to use the verified plan")
			} else {
				assert.Empty(t, info.PlanFile)
				assert.False(t, info.UseTerraformPlan)
			}
		})
	}
}

// TestBeforeWithWriters_BulkDeployVerifiesWithNodeAuth is a regression test for
// bulk deploy verification running with another node's credentials: Before
// injects the last aggregate auth context into the node info for its hooks, and
// the fresh verification plan used that injected context instead of the node's
// own. The node info must keep the injected context afterwards, because After
// receives the same info and reuses it for its hooks.
func TestBeforeWithWriters_BulkDeployVerifiesWithNodeAuth(t *testing.T) {
	lastAuthContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "previous-node"}}
	lastAuthManager := "previous-node-auth-manager"
	nodeAuthContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "this-node"}}
	nodeAuthManager := "this-node-auth-manager"

	tests := []struct {
		name            string
		nodeAuthContext *schema.AuthContext
		nodeAuthManager any
		wantHookContext *schema.AuthContext
		wantHookManager any
	}{
		{
			name:            "node without own auth verifies without the injected context",
			wantHookContext: lastAuthContext,
			wantHookManager: lastAuthManager,
		},
		{
			name:            "node with own auth verifies with it",
			nodeAuthContext: nodeAuthContext,
			nodeAuthManager: nodeAuthManager,
			wantHookContext: nodeAuthContext,
			wantHookManager: nodeAuthManager,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withoutCIDetection(t)
			info := writeTempAtmosProjectWithPlanfiles(t, storagePlanfilesYAML)
			info.VerifyPlanMode = schema.PlanfileVerifyFail
			info.ComponentType = cfg.TerraformComponentType
			info.AuthContext = tt.nodeAuthContext
			info.AuthManager = tt.nodeAuthManager
			stored := writeStoredPlan(t, info)

			e.SetLastAuthContext(lastAuthContext, lastAuthManager)
			t.Cleanup(e.ClearLastAuthContext)

			var verifyAuthContext *schema.AuthContext
			var verifyAuthManager any
			calls := stubVerifyPlanfile(t, func(info *schema.ConfigAndStacksInfo, storedPlanFile string) error {
				verifyAuthContext = info.AuthContext
				verifyAuthManager = info.AuthManager
				return fakeVerifyMatch(info, storedPlanFile)
			})

			nodeHooks := &terraformNodeHooks{cmd: newHookTestCmd(), subCommand: "deploy", beforeEvent: hooks.BeforeTerraformDeploy}
			err := nodeHooks.BeforeWithWriters(context.Background(), info, schema.ComponentNodeHookWriters{})

			require.NoError(t, err)
			require.Len(t, *calls, 1, "the stored plan must be verified exactly once")
			assert.Same(t, tt.nodeAuthContext, verifyAuthContext, "verification must run with the node's own auth context, not the injected one")
			assert.Equal(t, tt.nodeAuthManager, verifyAuthManager, "verification must run with the node's own auth manager, not the injected one")
			assert.Same(t, tt.wantHookContext, info.AuthContext, "the node info must keep the hook auth context that After reuses")
			assert.Equal(t, tt.wantHookManager, info.AuthManager, "the node info must keep the hook auth manager that After reuses")
			assert.Equal(t, freshPlanPath(stored), info.PlanFile, "the verified fresh plan must reach the node info")
			assert.True(t, info.UseTerraformPlan, "apply must be told to use the verified plan")
		})
	}
}
