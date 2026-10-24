package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestBackendTargetAuthReachesEveryOperation verifies each backend verb stops before AWS calls when target authentication fails.
func TestBackendTargetAuthReachesEveryOperation(t *testing.T) {
	for _, operation := range []string{"create", "exists", "delete", "describe", "list"} {
		t.Run(operation, func(t *testing.T) {
			original := resolveTargetAuth
			t.Cleanup(func() { resolveTargetAuth = original })
			targetAuth := map[string]any{"identity": "dev"}
			params := &CreateBackendParams{
				AtmosConfig: &schema.AtmosConfiguration{}, Component: "demo", Stack: "test", Target: "artifacts",
				RequestedIdentity: "cli-override",
				AuthContext:       &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "component"}},
				ComponentConfig: map[string]any{"provision": map[string]any{"targets": map[string]any{
					"artifacts": map[string]any{"kind": "aws/s3", "bucket": "bucket", "region": "us-east-2", "auth": targetAuth},
				}}},
			}
			called := false
			resolveTargetAuth = func(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, targetName string, block map[string]any, requested string) (*schema.ConfigAndStacksInfo, error) {
				called = true
				assert.Same(t, params.AtmosConfig, ac)
				assert.Equal(t, "test", info.Stack)
				assert.Equal(t, "demo", info.ComponentFromArg)
				assert.Equal(t, "artifacts", targetName)
				assert.Equal(t, "cli-override", requested)
				assert.Equal(t, targetAuth, block["auth"])
				return nil, errUtils.ErrAuthenticationFailed
			}
			p := &defaultProvisioner{}
			var err error
			switch operation {
			case "create":
				err = p.CreateBackend(t.Context(), params)
			case "exists":
				_, err = p.BackendExists(t.Context(), params)
			case "delete":
				err = p.DeleteBackend(t.Context(), &DeleteBackendParams{CreateBackendParams: *params})
			case "describe":
				err = p.DescribeBackend(t.Context(), &DescribeBackendParams{CreateBackendParams: *params})
			case "list":
				err = p.ListBackends(t.Context(), &ListBackendsParams{
					AtmosConfig: params.AtmosConfig, Component: params.Component, Stack: params.Stack,
					RequestedIdentity: params.RequestedIdentity, AuthContext: params.AuthContext, ComponentConfig: params.ComponentConfig,
				})
			}
			require.True(t, called)
			require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed, "failed target auth must stop before bucket operations")
			assert.Equal(t, "component", params.AuthContext.AWS.Profile)
		})
	}
}
