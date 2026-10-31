package provider

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestDiscoveryRejectsScopeAndConfiguration(t *testing.T) {
	for _, name := range []string{"provider", "settings", "empty scope", "imports", "unknown option", "non-native mode"} {
		t.Run(name, func(t *testing.T) {
			base := componentRequest(t)
			req := &pluginv1.DiscoverComponentsRequest{RootDirectory: base.RootDirectory, Provider: base.Provider, ProviderConfiguration: base.ProviderConfiguration, Operation: base.Operation, IncludePaths: []string{"live/**"}}
			switch name {
			case "provider":
				req.Provider = "other"
			case "settings":
				req.ProviderConfiguration = nil
			case "empty scope":
				req.IncludePaths = nil
			case "imports":
				req.Imports = []string{"terragrunt://live"}
			case "unknown option":
				req.ProviderConfiguration, _ = encode(map[string]any{"binary": "terragrunt"})
			case "non-native mode":
				req.ProviderConfiguration, _ = encode(map[string]any{"mode": "cli"})
			}
			p := New(func(context.Context, model.Request) (*model.Resolution, error) {
				t.Fatal("invalid requests must not evaluate HCL")
				return nil, nil
			})
			_, err := p.Discover(t.Context(), req)
			require.Error(t, err)
		})
	}
}

func TestDiscoveryRejectsUnrepresentableInventory(t *testing.T) {
	for _, name := range []string{"unit outside root", "module outside root", "missing module", "generation outside root", "watch outside root", "dependency outside scope", "evaluator failure"} {
		t.Run(name, func(t *testing.T) {
			base := componentRequest(t)
			outside := t.TempDir()
			unit := model.DiscoveredUnit{Stack: "live", Name: "app", Config: model.Resolution{Path: filepath.Join(base.RootDirectory, "live", "app", "terragrunt.hcl"), Folders: []string{filepath.Join(base.RootDirectory, "module")}}}
			switch name {
			case "unit outside root":
				unit.Config.Path = filepath.Join(outside, "terragrunt.hcl")
			case "module outside root":
				unit.Config.Folders = []string{outside}
			case "missing module":
				unit.Config.Folders = nil
			case "generation outside root":
				unit.GenerationRoot = filepath.Join(outside, "terragrunt.stack.hcl")
			case "watch outside root":
				unit.Config.Files = []string{filepath.Join(outside, "secret.hcl")}
			case "dependency outside scope":
				unit.Config.Dependencies = []string{filepath.Join(base.RootDirectory, "other", "terragrunt.hcl")}
			}
			p := New(func(context.Context, model.Request) (*model.Resolution, error) {
				if name == "evaluator failure" {
					return nil, context.Canceled
				}
				return &model.Resolution{Inventory: []model.DiscoveredUnit{unit}}, nil
			})
			result, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: base.RootDirectory, Provider: base.Provider, ProviderConfiguration: base.ProviderConfiguration, Operation: base.Operation, IncludePaths: []string{"live/**"}})
			if name == "evaluator failure" {
				require.Nil(t, result)
				require.Equal(t, codes.Canceled, status.Code(err))
				return
			}
			// A unit that cannot be represented is reported on its own and never hides healthy units.
			require.NoError(t, err)
			if name == "dependency outside scope" {
				return
			}
			require.Empty(t, result.Components)
			require.Len(t, result.Diagnostics, 1)
			detail := result.Diagnostics[0].Detail
			require.Equal(t, "app", detail.Unit)
			require.Equal(t, "live", detail.Stack)
			require.Equal(t, "discover", detail.Operation)
			require.NotEqual(t, pluginv1.Severity_SEVERITY_WARNING, detail.Severity)
		})
	}
}

func TestPreparationWithoutInputsAndDefaultState(t *testing.T) {
	req := componentRequest(t)
	req.Invocation = nil
	var err error
	req.Configuration, err = encode(map[string]any{})
	require.NoError(t, err)
	p := New(func(_ context.Context, request model.Request) (*model.Resolution, error) {
		require.Equal(t, "terraform", request.Tofu)
		require.Empty(t, request.Environment)
		return &model.Resolution{Source: filepath.Join(req.RootDirectory, "module"), WorkingDir: filepath.Join(req.RootDirectory, "cache")}, nil
	})
	result, err := p.Prepare(t.Context(), req)
	require.NoError(t, err)
	inputs, err := component.Decode(result.Inputs)
	require.NoError(t, err)
	require.False(t, inputs.IsNull())
	require.True(t, inputs.Type().IsObjectType())
	require.Empty(t, inputs.AsValueMap())
	require.Equal(t, "terraform", result.Engine)
	require.Equal(t, "default", result.State.Workspace)
	require.Equal(t, "local", result.State.BackendType)
}

func TestStateAndPreparationRejectEscapedProvenance(t *testing.T) {
	req := componentRequest(t)
	var err error
	req.Configuration, err = encode(map[string]any{})
	require.NoError(t, err)
	p := New(func(context.Context, model.Request) (*model.Resolution, error) {
		return &model.Resolution{Inputs: map[string]any{}, Files: []string{filepath.Join(filepath.Dir(req.RootDirectory), "outside.hcl")}}, nil
	})
	_, err = p.Prepare(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = p.Resolve(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	req.Source = "../outside"
	_, err = p.Resolve(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	p = New(func(context.Context, model.Request) (*model.Resolution, error) { return nil, context.Canceled })
	req.Source = "live/unit/terragrunt.hcl"
	_, err = p.Resolve(t.Context(), req)
	require.Equal(t, codes.Canceled, status.Code(err))
	req.Configuration, err = encode(map[string]any{"settings": map[string]any{"terragrunt": map[string]any{"generation_root": "../escape"}}})
	require.NoError(t, err)
	_, err = p.Prepare(t.Context(), req)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
