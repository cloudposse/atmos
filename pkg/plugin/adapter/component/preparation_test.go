package component

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func TestPreparationTransportsUnknownValuesButRejectsUnknownInputs(t *testing.T) {
	p := fixtureProvider(t)
	root := t.TempDir()
	number, err := cty.ParseNumberVal("9007199254740993123456789.123456789")
	require.NoError(t, err)
	request := &pluginv1.ComponentRequest{
		RootDirectory: root, Address: fixtureAddress(),
		Operation:     &pluginv1.ComponentOperation{Command: "plan", Mode: "native"},
		Configuration: wireValue(t, cty.ObjectVal(map[string]cty.Value{"exact": number, "unknown": cty.UnknownVal(cty.String)})),
	}
	request.Configuration.SensitivePaths = []*pluginv1.ComponentValuePath{{}}
	before := proto.Clone(request)
	state, err := p.ResolveState(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "development", state.State.Workspace)
	validated, err := p.Validate(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, validated.Diagnostics)
	require.NoFileExists(t, filepath.Join(root, "preparation.pb"))

	// The fixture echoes the transported configuration as inputs, which must be fully known.
	prepared, err := p.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	require.Nil(t, prepared)
	require.True(t, proto.Equal(before, request), "rejection must not mutate caller input")
	seen := &pluginv1.ComponentRequest{}
	raw, err := os.ReadFile(filepath.Join(root, "preparation.pb"))
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(raw, seen))
	require.True(t, proto.Equal(request.Configuration, seen.Configuration), "transport preserves unknown values and sensitive paths")
	require.NoFileExists(t, filepath.Join(root, "backend.tf"), "rejected preparation must not materialize artifacts")
}

func TestProviderIAMProbeAcceptsRoleOnlyResponse(t *testing.T) {
	p := fixtureProvider(t)
	result, err := p.Prepare(t.Context(), &pluginv1.ComponentRequest{
		RootDirectory: t.TempDir(), Address: fixtureAddress(),
		Operation: &pluginv1.ComponentOperation{Command: "plan", Mode: "iam-probe"},
	})
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::1:role/test", result.GetIam().GetRole())
	require.ErrorIs(t, checkPreparation(result), errUtils.ErrPluginProtocol)
}

func TestIAMProbeRejectsModulePreparation(t *testing.T) {
	t.Parallel()
	require.NoError(t, checkPreparationResponse(&pluginv1.PrepareComponentResponse{}, "iam-probe"))
	require.ErrorIs(t, checkPreparationResponse(nil, "iam-probe"), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, checkPreparationResponse(&pluginv1.PrepareComponentResponse{ModuleSource: "./module"}, "iam-probe"), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, checkPreparationResponse(&pluginv1.PrepareComponentResponse{Diagnostics: []*pluginv1.Diagnostic{{Message: "failure"}}}, "iam-probe"), errUtils.ErrPluginInvalidConfig)
}

func TestPreparationIAMProbeResponse(t *testing.T) {
	t.Parallel()
	response := &pluginv1.PrepareComponentResponse{Iam: &pluginv1.ComponentIam{}}
	require.NoError(t, checkPreparationResponse(response, "iam-probe"))
	require.NoError(t, checkPreparationResponse(&pluginv1.PrepareComponentResponse{}, "iam-probe"))
	require.ErrorIs(t, checkPreparationResponse(response, "native"), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, checkPreparationResponse(nil, "iam-probe"), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, checkPreparationResponse(&pluginv1.PrepareComponentResponse{Diagnostics: []*pluginv1.Diagnostic{{Message: "failure"}}}, "iam-probe"), errUtils.ErrPluginInvalidConfig)
	response.Files = []*pluginv1.ComponentGeneratedFile{{Path: "backend.tf", IfExists: "overwrite"}}
	require.ErrorIs(t, checkPreparationResponse(response, "iam-probe"), errUtils.ErrPluginProtocol)
}
