package terraform_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/exec"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// TestNestedTerraformSkip checks real nested descriptions without Terraform or a secret backend.
func TestNestedTerraformSkip(t *testing.T) {
	for _, deferredMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "eager", true: "deferred"}[deferredMode], func(t *testing.T) {
			testNestedTerraformSkip(t, deferredMode)
		})
	}
}

func testNestedTerraformSkip(t *testing.T, deferredMode bool) {
	t.Helper()
	for _, tag := range []string{"terraform.output", "terraform.state"} {
		t.Run(tag, func(t *testing.T) {
			config, mockStore := setupNestedSkipReference(t)
			if deferredMode {
				authdeferred.ConfigureAuth(config, "false")
			}
			path := filepath.Join(config.BasePath, "stacks", "dev.yaml")
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(content), "!terraform.output producer credential", "!"+tag+" producer credential")), 0o600))
			calls := 0
			mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").DoAndReturn(func(_, _, _ string) (any, error) {
				calls++
				return nestedSkipSecret, nil
			}).AnyTimes()
			describe := func(skip []string) any {
				sections, err := exec.ExecuteDescribeComponent(&exec.ExecuteDescribeComponentParams{
					AtmosConfig: config, Component: "consumer", Stack: "dev", ProcessTemplates: true,
					ProcessYamlFunctions: true, ResolveSecrets: true, Skip: skip,
				})
				require.NoError(t, err)
				return sections["vars"].(map[string]any)["credential"]
			}
			assert.Equal(t, "!secret CREDENTIAL", describe([]string{"secret"}))
			assert.Zero(t, calls, "nested skip must not read secrets")
			assert.Equal(t, nestedSkipSecret, describe(nil), "skipped outputs must not poison normal lookups")
			assert.Positive(t, calls)
			before := calls
			assert.Equal(t, "!secret CREDENTIAL", describe([]string{"secret"}), "normal outputs must not bypass skip")
			assert.Equal(t, before, calls)
		})
	}
}

const nestedSkipSecret = "synthetic-nested-skip-secret"

// setupNestedSkipReference copies the scenario so each mode can alter its reference independently.
// Only the secret backend is mocked; descriptions and cross-component resolution use production code.
func setupNestedSkipReference(t *testing.T) (*schema.AtmosConfiguration, *store.MockStore) {
	t.Helper()
	fixture, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "terraform-nested-skip"))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(fixture)))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	t.Setenv("ATMOS_MASK", "true")
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	mockStore := store.NewMockStore(gomock.NewController(t))
	atmosConfig.StoresConfig = store.StoresConfig{"test-secrets": {Type: "test", Secret: true}}
	atmosConfig.Stores = store.StoreRegistry{"test-secrets": mockStore}
	iolib.Reset()
	require.NoError(t, iolib.Initialize())
	iolib.ApplyMaskingConfig(&iolib.Config{AtmosConfig: atmosConfig})
	reset := func() {
		exec.ResetStateCache()
		tfoutput.ResetOutputsCache()
	}
	reset()
	t.Cleanup(func() { reset(); iolib.Reset() })
	return &atmosConfig, mockStore
}
