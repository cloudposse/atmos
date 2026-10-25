package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestComponentAdmissionAgentProductionPolicy(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "policy-library", "component-deny-agent-production-deploy"))
	require.NoError(t, err)
	for _, variant := range []string{"", filepath.Join("variants", "starlark")} {
		t.Run(variant, func(t *testing.T) {
			t.Setenv("ATMOS_POLICY_DIR", filepath.Join(root, variant))
			for _, marker := range []string{"CLAUDECODE", "CODEX_THREAD_ID"} {
				t.Run(marker, func(t *testing.T) {
					t.Setenv("CLAUDECODE", "")
					t.Setenv("CODEX_THREAD_ID", "")
					t.Setenv(marker, "1")
					for _, command := range []string{"apply", "deploy"} {
						info := &schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: command, ComponentFromArg: "app", Stack: "acme-ue2-prod"}
						require.ErrorIs(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info), errUtils.ErrPolicyDenied)
						info.Stack = "acme-ue2-dev"
						require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
						info.Stack, info.SubCommand = "acme-ue2-prod", "plan"
						require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
					}
				})
			}
			t.Setenv("CLAUDECODE", "")
			t.Setenv("CODEX_THREAD_ID", "")
			info := &schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "apply", ComponentFromArg: "app", Stack: "prod"}
			require.NoError(t, enforceComponentAdmission(&schema.AtmosConfiguration{}, info))
		})
	}
}
