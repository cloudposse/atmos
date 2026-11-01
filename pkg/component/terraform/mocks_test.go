package terraform

import (
	fmt "fmt"
	os "os"
	filepath "path/filepath"
	testing "testing"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	schema "github.com/cloudposse/atmos/pkg/schema"
	refcache "github.com/cloudposse/atmos/pkg/terraform/refcache"
	testhelpers "github.com/cloudposse/atmos/tests/testhelpers"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func TestMergeRealOverMocks(t *testing.T) {
	mocks := map[string]any{
		"vpc_id":  "vpc-mock",
		"network": map[string]any{"cidr": "10.0.0.0/16", "tags": map[string]any{"env": "mock", "team": "mock"}},
		"subnets": []any{"mock-a", "mock-b"},
		"only":    "mock-only",
	}
	real := map[string]any{
		"vpc_id":  "vpc-real",
		"network": map[string]any{"name": "real", "tags": map[string]any{"env": "real"}},
		"subnets": []any{"real-a"},
		"nulled":  nil,
	}

	merged := mergeRealOverMocks(mocks, real)

	assert.Equal(t, map[string]any{
		"vpc_id":  "vpc-real",
		"network": map[string]any{"name": "real", "cidr": "10.0.0.0/16", "tags": map[string]any{"env": "real", "team": "mock"}},
		"subnets": []any{"real-a"},
		"only":    "mock-only",
		"nulled":  nil,
	}, merged)

	// Neither input is mutated by the merge, and the result does not alias their nested maps.
	assert.Equal(t, map[string]any{"cidr": "10.0.0.0/16", "tags": map[string]any{"env": "mock", "team": "mock"}}, mocks["network"])
	assert.Equal(t, map[string]any{"name": "real", "tags": map[string]any{"env": "real"}}, real["network"])
	merged["network"].(map[string]any)["cidr"] = "changed"
	assert.Equal(t, "10.0.0.0/16", mocks["network"].(map[string]any)["cidr"])
}

func TestResolvedFromMocks(t *testing.T) {
	mocks := map[string]any{"vpc_id": "vpc-mock", "network": map[string]any{"cidr": "10.0.0.0/16"}}
	real := map[string]any{"vpc_id": "vpc-real", "network": map[string]any{"name": "real"}}
	tests := []struct {
		output string
		want   bool
	}{
		{output: "vpc_id", want: false},
		{output: ".network.cidr", want: true},
		{output: ".network.name", want: false},
		{output: "missing", want: false},
		{output: `.network.cidr // "x"`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			assert.Equal(t, tt.want, resolvedFromMocks(mocks, real, tt.output))
		})
	}
}

func TestMockPathExists(t *testing.T) {
	mocks := map[string]any{
		"vpc_id":   "vpc-1",
		"nullable": nil,
		"network":  map[string]any{"cidr": "10.0.0.0/16"},
	}
	tests := []struct {
		output         string
		wantExists     bool
		wantDecidable  bool
		wantOutputFlag bool
	}{
		{output: "vpc_id", wantExists: true, wantDecidable: true, wantOutputFlag: true},
		{output: "nullable", wantExists: true, wantDecidable: true, wantOutputFlag: true},
		{output: "missing", wantExists: false, wantDecidable: true, wantOutputFlag: false},
		{output: ".network.cidr", wantExists: true, wantDecidable: true, wantOutputFlag: true},
		{output: ".network.missing", wantExists: false, wantDecidable: true, wantOutputFlag: false},
		{output: ".vpc_id.deeper", wantExists: false, wantDecidable: true, wantOutputFlag: false},
		{output: `.missing // "x"`, wantExists: false, wantDecidable: false, wantOutputFlag: true},
	}
	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			exists, decidable := mockPathExists(mocks, tt.output)
			assert.Equal(t, tt.wantExists, exists)
			assert.Equal(t, tt.wantDecidable, decidable)
			assert.Equal(t, tt.wantOutputFlag, mockOutputExists(mocks, tt.output))
		})
	}
}

// TestResolveTerraformMockOutputInvalidMode verifies that an unknown
// components.terraform.mocks.mode (only reachable through atmos.yaml, since the
// env var and flag paths validate it) fails loudly under --use-mocks instead of
// silently matching neither mode, and is ignored when mocks are off.
func TestResolveTerraformMockOutputInvalidMode(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Mocks.Mode = schema.TerraformMocksMode("bogus")

	t.Run("use-mocks on errors", func(t *testing.T) {
		value, handled, err := testExecutor(t).ResolveTerraformMockOutput(atmosConfig, &schema.ConfigAndStacksInfo{UseMocks: true}, "dev", "vpc", "vpc_id")
		require.ErrorIs(t, err, errUtils.ErrInvalidMocksMode)
		assert.True(t, handled)
		assert.Nil(t, value)
		assert.Contains(t, err.Error(), `"bogus"`)
	})

	t.Run("use-mocks off ignores the mode", func(t *testing.T) {
		value, handled, err := testExecutor(t).ResolveTerraformMockOutput(atmosConfig, &schema.ConfigAndStacksInfo{UseMocks: false}, "dev", "vpc", "vpc_id")
		require.NoError(t, err)
		assert.False(t, handled)
		assert.Nil(t, value)
	})
}

func TestTerraformMocksModeGuards(t *testing.T) {
	assert.Equal(t, schema.TerraformMocksModeFallback, terraformMocksMode(nil))

	invalid := &schema.AtmosConfiguration{}
	invalid.Components.Terraform.Mocks.Mode = "bogus"
	require.NoError(t, validateTerraformMocksMode(invalid, nil), "a nil stack info means mocks are off")
}

func authContextForProfile(profile string) *schema.AuthContext {
	return &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: profile}}
}

func writeTerraformStateOutput(t *testing.T, path, value string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`{"version":4,"outputs":{"id":{"value":%q,"type":"string"}}}`, value)), 0o600))
}

func setupDeferredCacheTarget(t *testing.T) (*schema.AtmosConfiguration, *authdeferred.MockAuthFactory, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"atmos.yaml": `base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["**/*"]
`,
		"components/terraform/target/main.tf": "",
		"stacks/dev.yaml": `components:
  terraform:
    target:
      backend_type: local
      backend:
        local:
          path: cached.tfstate
      vars: {}
`,
	} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	factory := authdeferred.NewMockAuthFactory(gomock.NewController(t))
	ac.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{Factory: factory})
	return &ac, factory, filepath.Join(dir, "components", "terraform", "target", "cached.tfstate")
}

func TestInvalidateTerraformStateCacheRemovesAllCallerScopes(t *testing.T) {
	ac, _, path := setupDeferredCacheTarget(t)
	ac.AuthManager = nil
	ResetStateCache()
	t.Cleanup(ResetStateCache)
	scopes := []*schema.AuthContext{authContextForProfile("first"), authContextForProfile("second"), nil}
	writeTerraformStateOutput(t, path, "before")
	for _, scope := range scopes {
		value, err := testExecutor(t).GetTerraformState(ac, "!terraform.state", "dev", "target", "id", false, scope, nil)
		require.NoError(t, err)
		require.Equal(t, "before", value)
	}

	writeTerraformStateOutput(t, path, "after")
	invalidateTerraformStateCache("dev", "target")
	for _, scope := range scopes {
		value, err := testExecutor(t).GetTerraformState(ac, "!terraform.state", "dev", "target", "id", false, scope, nil)
		require.NoError(t, err)
		require.Equal(t, "after", value)
	}
}

func TestInvalidateTerraformStateCacheAllPartitions(t *testing.T) {
	ResetStateCache()
	t.Cleanup(ResetStateCache)
	keys := []refcache.Key{}
	for _, basePath := range []string{"head", "base"} {
		for _, configPath := range []string{"config-first", "config-second"} {
			for _, maskOnly := range []bool{false, true} {
				for _, authDisabled := range []bool{false, true} {
					key := refcache.NewKey(&schema.AtmosConfiguration{BasePath: basePath, CliConfigPath: configPath}, "dev", "target")
					key.Inspection = maskOnly
					key.Auth = refcache.AuthScope(nil, nil, authDisabled)
					keys = append(keys, key)
					terraformStateCache.Store(key, terraformStateNotProvisionedCacheEntry{})
				}
			}
		}
	}
	unrelated := refcache.NewKey(&schema.AtmosConfiguration{}, "other", "target")
	terraformStateCache.Store(unrelated, terraformStateNotProvisionedCacheEntry{})
	invalidateTerraformStateCache("dev", "target")
	for _, key := range keys {
		_, exists := terraformStateCache.Load(key)
		require.False(t, exists)
	}
	_, exists := terraformStateCache.Load(unrelated)
	require.True(t, exists, "another stack must retain its cached state")
}

func setupTerraformYamlFunctionSandbox(t *testing.T, workDir string) {
	t.Helper()

	sandbox, err := testhelpers.SetupSandbox(t, workDir)
	if err != nil {
		t.Fatalf("Failed to setup sandbox: %v", err)
	}
	t.Cleanup(sandbox.Cleanup)

	for k, v := range sandbox.GetEnvironmentVariables() {
		t.Setenv(k, v)
	}
}
