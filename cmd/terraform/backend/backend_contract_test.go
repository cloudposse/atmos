package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestUnsupportedBackendInspectionDoesNotAuthenticate(t *testing.T) {
	for _, command := range []*cobra.Command{describeCmd, listCmd} {
		t.Run(command.Name(), func(t *testing.T) {
			setupViperForTest(t, map[string]any{"stack": "dev", "identity": "not-an-identity", "interactive": false})
			t.Cleanup(ResetDependencies)
			SetConfigInitializer(NewMockConfigInitializer(gomock.NewController(t)))
			SetProvisioner(nil)
			for _, name := range []string{"stack", "identity"} {
				command.Flags().Lookup(name).Changed = false
			}
			args := []string(nil)
			if command == describeCmd {
				args = []string{"probe"}
			}
			require.ErrorIs(t, command.RunE(command, args), errUtils.ErrNotImplemented)
			require.Contains(t, command.Long, "not yet implemented")
		})
	}
	require.Error(t, listCmd.Args(listCmd, []string{"unexpected-component"}))
	require.ErrorIs(t, executeListCommandWithValues("", "", "table", false), errUtils.ErrRequiredFlagNotProvided)
	require.ErrorIs(t, executeDescribeCommandWithValues("probe", "", "", "yaml", promptedFlags{}), errUtils.ErrRequiredFlagNotProvided)
}

func TestBackendEnvironmentResolutionIsScoped(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ['*.yaml']
  name_template: '{{ .vars.stage }}'
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "dev.yaml"), []byte(`vars: {stage: dev}
components:
  terraform:
    probe:
      env: {FIELD_BUCKET: component-bucket}
      vars:
        untouched: !terraform.output missing dev value
      backend_type: s3
      backend:
        s3:
          bucket: !env FIELD_BUCKET
          key: !env FIELD_KEY fallback.tfstate
          region: us-east-1
          endpoints:
            s3: !env FIELD_ENDPOINT
          account: !terraform.state missing dev account
          workspace_key_prefix: '!env FIELD_BUCKET'
          fallback: !env FIELD_TEST_UNSET default-value
`), 0o600))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("FIELD_BUCKET", "process-bucket")
	t.Setenv("FIELD_KEY", "reviewed.tfstate")
	t.Setenv("FIELD_ENDPOINT", "http://127.0.0.1:4566")
	t.Setenv("FIELD_TEST_UNSET", "")
	require.NoError(t, os.Unsetenv("FIELD_TEST_UNSET"))
	config, err := CreateDescribeComponentFunc(nil)("probe", "dev")
	require.NoError(t, err)
	backend := config["backend"].(map[string]any)
	require.Equal(t, "component-bucket", backend["bucket"])
	require.Equal(t, "reviewed.tfstate", backend["key"])
	require.Equal(t, "http://127.0.0.1:4566", backend["endpoints"].(map[string]any)["s3"])
	account, ok := yamlsource.Decode(backend["account"].(string))
	require.True(t, ok, "state lookup must stay deferred")
	require.Equal(t, "!terraform.state", account.Tag)
	require.Equal(t, "!env FIELD_BUCKET", backend["workspace_key_prefix"], "ordinary strings must retain explicit-tag policy")
	require.Equal(t, "default-value", backend["fallback"])
	// Explicit auth disable still bypasses any configured identity selection.
	loaded, context, err := InitConfigAndAuth("probe", "dev", cfg.IdentityFlagDisabledValue, false, false)
	require.NoError(t, err)
	require.Nil(t, context)
	// Config initialization intentionally does not load stacks; the production
	// callback must still perform its own stack discovery before evaluation.
	resolved, err := createDescribeComponentFunc(loaded, nil)("probe", "dev")
	require.NoError(t, err)
	require.Equal(t, "component-bucket", resolved["backend"].(map[string]any)["bucket"])
}

func TestBackendEnvironmentRejectsMalformedTag(t *testing.T) {
	config := map[string]any{"backend": map[string]any{"bucket": yamlsource.Source{Tag: "!env"}.Encode()}}
	_, err := resolveBackendEnvironment(&schema.AtmosConfiguration{}, config, "probe", "dev")
	require.ErrorIs(t, err, u.ErrInvalidAtmosYAMLFunction)
}

var _ = schema.ConfigAndStacksInfo{ComponentEnvSection: nil}
