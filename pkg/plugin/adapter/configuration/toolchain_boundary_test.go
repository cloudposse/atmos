package configuration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// preparedRequest runs Session.Prepare against a recording provider and returns the exact
// request the plugin would receive. The request is then round-tripped through the wire
// encoding, because the environment crosses a process boundary in production.
func preparedRequest(t *testing.T, info *schema.ConfigAndStacksInfo) *pluginv1.ComponentRequest {
	t.Helper()
	config := enabledConfig(t)
	provider := NewMockProvider(gomock.NewController(t))
	provider.EXPECT().ComponentType().Return("terraform").AnyTimes()
	component := unit(t, "app")
	provider.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(&pluginv1.DiscoverComponentsResponse{Components: []*pluginv1.DiscoveredComponent{component}}, nil)
	inputs, err := adapter.EncodeObject(map[string]any{})
	require.NoError(t, err)
	backend, err := adapter.EncodeObject(map[string]any{"path": "state.tfstate"})
	require.NoError(t, err)
	var captured *pluginv1.ComponentRequest
	provider.EXPECT().Prepare(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error) {
		wire, err := proto.Marshal(request)
		require.NoError(t, err)
		captured = &pluginv1.ComponentRequest{}
		require.NoError(t, proto.Unmarshal(wire, captured))
		return &pluginv1.PrepareComponentResponse{Engine: "opentofu", ModuleSource: "/module", Inputs: inputs, State: &pluginv1.ComponentStateIdentity{BackendType: "local", Backend: backend, Workspace: "default", WorkingDirectory: config.BasePathAbsolute}}, nil
	})
	session, err := New(t.Context(), config, "plan", func(context.Context, []string) (map[string]Provider, error) {
		return map[string]Provider{"terragrunt": provider}, nil
	})
	require.NoError(t, err)
	info.Stack, info.ComponentFromArg, info.SubCommand = "live", "app", "plan"
	info.ComponentSettingsSection = map[string]any{"configuration": true}
	_, err = session.Prepare(t.Context(), info)
	require.NoError(t, err)
	require.NotNil(t, captured)
	return captured
}

// The toolchain PATH, the pinned executable, and the identity environment must survive
// the host-to-plugin boundary exactly as the host assembled them, and nothing ambient may
// be added once an identity has produced an authoritative environment.
func TestToolchainEnvironmentCrossesThePluginBoundary(t *testing.T) {
	toolchain := filepath.Join(t.TempDir(), "bin")
	ambient := t.TempDir()
	t.Setenv("PATH", ambient)
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "ambient-token-must-not-return")
	t.Setenv("ATMOS_AMBIENT_ONLY", "ambient")
	pinned := filepath.Join(toolchain, "tofu")
	toolchainPath := toolchain + string(os.PathListSeparator) + ambient

	request := preparedRequest(t, &schema.ConfigAndStacksInfo{
		Command:  pinned,
		Identity: "production",
		// The sanitized environment is what the selected identity produced.
		SanitizedEnv: []string{"PATH=" + ambient, "AWS_PROFILE=production", "SHARED=identity"},
		// Component environment follows, and the toolchain PATH is appended last.
		ComponentEnvList: []string{
			"SHARED=component", "CUSTOM_EMPTY=", "ATMOS_TERRAFORM_BINARY=stack-must-not-win",
			"PATH=" + toolchainPath,
		},
		TerraformWorkspace: "stage",
	})
	environment := request.Invocation.Environment

	require.Equal(t, toolchainPath, environment["PATH"], "the toolchain PATH must reach the plugin intact")
	require.Equal(t, pinned, environment["ATMOS_TERRAFORM_BINARY"], "the pinned executable is host-owned and cannot be overridden by stack env")
	require.Equal(t, "production", environment["AWS_PROFILE"])
	require.Equal(t, "component", environment["SHARED"], "component environment wins over the identity environment")
	value, present := environment["CUSTOM_EMPTY"]
	require.True(t, present, "an explicitly empty value must be preserved")
	require.Empty(t, value)
	require.Equal(t, "stage", environment["TF_WORKSPACE"])
	for _, leaked := range []string{"AWS_WEB_IDENTITY_TOKEN_FILE", "ATMOS_AMBIENT_ONLY"} {
		require.NotContains(t, environment, leaked, "ambient values must not return behind an identity")
	}
}

// Without an identity the ambient environment is the base, the component PATH still wins,
// and two preparations in one process never share environment state.
func TestToolchainEnvironmentWithoutIdentityAndAcrossPreparations(t *testing.T) {
	ambient := t.TempDir()
	t.Setenv("PATH", ambient)
	t.Setenv("ATMOS_AMBIENT_ONLY", "ambient")
	first := preparedRequest(t, &schema.ConfigAndStacksInfo{Command: "/first/tofu", ComponentEnvList: []string{"PATH=/first/bin", "ONLY_FIRST=1"}})
	second := preparedRequest(t, &schema.ConfigAndStacksInfo{Command: "/second/tofu", ComponentEnvList: []string{"PATH=/second/bin"}})

	require.Equal(t, "/first/bin", first.Invocation.Environment["PATH"])
	require.Equal(t, "/first/tofu", first.Invocation.Environment["ATMOS_TERRAFORM_BINARY"])
	require.Equal(t, "1", first.Invocation.Environment["ONLY_FIRST"])
	require.Equal(t, "ambient", first.Invocation.Environment["ATMOS_AMBIENT_ONLY"])

	require.Equal(t, "/second/bin", second.Invocation.Environment["PATH"])
	require.Equal(t, "/second/tofu", second.Invocation.Environment["ATMOS_TERRAFORM_BINARY"])
	require.NotContains(t, second.Invocation.Environment, "ONLY_FIRST", "one preparation's environment must not leak into the next")
}
