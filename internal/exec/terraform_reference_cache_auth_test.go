package exec

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile guards for the caller credential fields that scope the shared caches.
var (
	_ = schema.ConfigAndStacksInfo{AuthContext: nil, AuthDisabled: false}
	_ = schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: ""}}
)

func authContextForProfile(profile string) *schema.AuthContext {
	return &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: profile}}
}

func writeTerraformStateOutput(t *testing.T, path, value string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`{"version":4,"outputs":{"id":{"value":%q,"type":"string"}}}`, value)), 0o600))
}

// Callers with different credentials must each read the backend; callers with equal
// credential content share one read. The backend file changes after every read, so a
// returned value identifies which read produced it.
func TestGetTerraformStateCacheSeparatesCallerCredentials(t *testing.T) {
	for _, maskOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("inspect=%t", maskOnly), func(t *testing.T) {
			ac, _, path := setupDeferredCacheTarget(t)
			ac.AuthManager = nil
			ResetStateCache()
			t.Cleanup(ResetStateCache)
			options := TerraformLookupOptions{SecretsMaskOnly: maskOnly}
			lookup := func(authContext *schema.AuthContext) any {
				value, err := GetTerraformState(ac, "!terraform.state", "dev", "target", "id", false, authContext, nil, options)
				require.NoError(t, err)
				return value
			}

			writeTerraformStateOutput(t, path, "read-1")
			require.Equal(t, "read-1", lookup(authContextForProfile("first")))

			writeTerraformStateOutput(t, path, "read-2")
			require.Equal(t, "read-2", lookup(authContextForProfile("second")), "different credentials must not reuse the first caller's read")
			require.Equal(t, "read-2", lookup(nil), "no credentials is its own scope")

			writeTerraformStateOutput(t, path, "read-3")
			require.Equal(t, "read-1", lookup(authContextForProfile("first")), "the first caller keeps its own cached read")
			require.Equal(t, "read-1", lookup(authContextForProfile("first")), "equal credential content shares one read")
			require.Equal(t, "read-2", lookup(authContextForProfile("second")))
		})
	}
}

// Invalidating a component removes the reads of every caller scope.
// atmos.Component() results are partitioned by the enclosing component's credentials and
// by whether authentication is disabled.
func TestComponentFuncCacheSeparatesCallerCredentials(t *testing.T) {
	tests := []struct {
		name string
		// first and second are the callers; sameScope reports whether they must share one execution.
		first, second *schema.ConfigAndStacksInfo
		sameScope     bool
	}{
		{
			name:      "different credentials",
			first:     &schema.ConfigAndStacksInfo{AuthContext: authContextForProfile("first")},
			second:    &schema.ConfigAndStacksInfo{AuthContext: authContextForProfile("second")},
			sameScope: false,
		},
		{
			name:      "credentials and none",
			first:     &schema.ConfigAndStacksInfo{AuthContext: authContextForProfile("first")},
			second:    &schema.ConfigAndStacksInfo{},
			sameScope: false,
		},
		{
			name:      "equal credential content",
			first:     &schema.ConfigAndStacksInfo{AuthContext: authContextForProfile("same")},
			second:    &schema.ConfigAndStacksInfo{AuthContext: authContextForProfile("same")},
			sameScope: true,
		},
		{
			name:      "auth disabled and enabled",
			first:     &schema.ConfigAndStacksInfo{AuthDisabled: true},
			second:    &schema.ConfigAndStacksInfo{AuthDisabled: false},
			sameScope: false,
		},
		{
			name:      "both auth disabled",
			first:     &schema.ConfigAndStacksInfo{AuthDisabled: true},
			second:    &schema.ConfigAndStacksInfo{AuthDisabled: true},
			sameScope: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, _, _ := setupDeferredCacheTarget(t)
			ac.AuthManager = nil
			componentFuncSyncMap.Clear()
			t.Cleanup(componentFuncSyncMap.Clear)
			executor := NewMockComponentFuncOutputsExecutor(gomock.NewController(t))
			original := componentFuncOutputsExecutor
			componentFuncOutputsExecutor = executor
			t.Cleanup(func() { componentFuncOutputsExecutor = original })

			executions := 0
			executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ *schema.AtmosConfiguration, _, _ string, _ map[string]any, _ *schema.AuthContext) (map[string]any, error) {
					executions++
					return map[string]any{"id": fmt.Sprintf("execution-%d", executions)}, nil
				},
			).AnyTimes()

			outputID := func(info *schema.ConfigAndStacksInfo) any {
				sections, err := componentFunc(t.Context(), ac, info, "target", "dev")
				require.NoError(t, err)
				return sections.(map[string]any)["outputs"].(map[string]any)["id"]
			}
			require.Equal(t, "execution-1", outputID(tt.first))
			if tt.sameScope {
				require.Equal(t, "execution-1", outputID(tt.second))
				require.Equal(t, 1, executions)
			} else {
				require.Equal(t, "execution-2", outputID(tt.second), "a different scope must execute again")
				require.Equal(t, "execution-1", outputID(tt.first), "the first scope keeps its own cached result")
				require.Equal(t, 2, executions)
			}
		})
	}
}
