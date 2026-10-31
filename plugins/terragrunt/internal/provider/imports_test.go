package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestImportsSelectExplicitFilesAndDirectories(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "live"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "live", "terragrunt.stack.hcl"), nil, 0o600))
	settings, err := encode(map[string]any{"enabled": true})
	require.NoError(t, err)
	request := &pluginv1.DiscoverComponentsRequest{RootDirectory: root, Provider: "terragrunt", ProviderConfiguration: settings, Operation: &pluginv1.ComponentOperation{Mode: "native"}, Imports: []string{"terragrunt://live", "terragrunt://live/terragrunt.stack.hcl"}}
	paths, err := importPaths(request)
	require.NoError(t, err)
	require.Equal(t, []string{"live/**", "live/terragrunt.stack.hcl"}, paths)
	p := New(func(_ context.Context, req model.Request) (*model.Resolution, error) {
		require.Equal(t, paths, req.IncludedPaths)
		return &model.Resolution{}, nil
	})
	_, err = p.Discover(t.Context(), request)
	require.NoError(t, err)
	for _, uri := range []string{"wrong://live", "terragrunt://../outside", "terragrunt://live?ref=main", "terragrunt://absent", "terragrunt://live/*", `terragrunt://live\child`} {
		request.Imports = []string{uri}
		_, err := importPaths(request)
		require.Error(t, err)
		_, err = p.Discover(t.Context(), request)
		require.Error(t, err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "live", "other.txt"), nil, 0o600))
	request.Imports = []string{"terragrunt://live/other.txt"}
	_, err = importPaths(request)
	require.Error(t, err)
	request.Imports = nil
	_, err = importPaths(request)
	require.Error(t, err)
}

func TestRemoteModuleSourceIsNotAFileWatch(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	path := filepath.Join(root, "live", "app", "terragrunt.hcl")
	remote := "git::https://example.com/modules.git//app?ref=v1.0.0"
	unit := model.DiscoveredUnit{Config: model.Resolution{Path: path, Source: remote, Files: []string{path}}}
	addresses := map[string]*pluginv1.ComponentAddress{path: {Stack: "live", Component: "app", ComponentType: "terraform"}}
	instance, err := discoveredUnit(model.Request{BasePath: root}, unit, addresses)
	require.NoError(t, err)
	var configuration map[string]any
	require.NoError(t, decode(instance.Configuration, &configuration))
	require.Equal(t, remote, configuration["source"])
	require.Len(t, instance.Sources, 1)
	require.Equal(t, "live/app/terragrunt.hcl", instance.Sources[0].Path)
}

func TestDerivedAddressNamesAnUnscopedUnit(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for path, want := range map[string]*pluginv1.ComponentAddress{
		filepath.Join(root, "live", "dev", "vpc", "terragrunt.hcl"): {Stack: "live/dev", Component: "vpc", ComponentType: "terraform"},
		filepath.Join(root, "vpc", "terragrunt.hcl"):                {Stack: "default", Component: "vpc", ComponentType: "terraform"},
		filepath.Join(root, "terragrunt.hcl"):                       {Stack: "default", Component: "root", ComponentType: "terraform"},
	} {
		got, err := derivedAddress(root, path)
		require.NoError(t, err)
		require.Equal(t, want.Stack, got.Stack)
		require.Equal(t, want.Component, got.Component)
		require.Equal(t, want.ComponentType, got.ComponentType)
	}
	_, err = derivedAddress(root, filepath.Join(filepath.Dir(root), "elsewhere", "terragrunt.hcl"))
	require.ErrorIs(t, err, ErrUnsupported)
}

func TestWarningDiagnosticsNeverFailTheCommand(t *testing.T) {
	diagnostics := warningDiagnostics([]model.Warning{{Code: model.WarningSymlinkedDirectory, Path: "live/linked", Message: "symlinked directory live/linked was not followed"}})
	require.Len(t, diagnostics, 1)
	require.Equal(t, pluginv1.Severity_SEVERITY_WARNING, diagnostics[0].Detail.Severity)
	require.Equal(t, model.WarningSymlinkedDirectory, diagnostics[0].Detail.Code)
	require.True(t, diagnostics[0].Detail.UserSafe)
	require.Equal(t, "live/linked", diagnostics[0].Detail.Location.File)
	require.Empty(t, warningDiagnostics(nil))
}
