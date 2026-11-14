package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/engine"
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestUnknownEvaluatorErrorKeepsItsTextHidden(t *testing.T) {
	err := rpcError(errors.New("password=hunter2"))
	detail, ok := component.FailureDetail(err)
	require.True(t, ok)
	require.False(t, detail.UserSafe)
	require.NotContains(t, status.Convert(err).Message(), "hunter2")
	require.Empty(t, detail.Hints, "the host adds the debug hint for withheld detail")
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestProblemKeepsItsFieldsAndRelativizesTheFile(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	problem := &model.Problem{Code: model.CodeSyntaxError, Summary: "Invalid", File: filepath.Join(root, "live", "dev", "terragrunt.hcl"), Line: 4, Column: 9, Safe: true, Detail: "comma", Hints: []string{"fix it"}}
	err = scoped(rpcError(problem), "discover", root, &pluginv1.ComponentAddress{Stack: "live/dev", Component: "vpc"})
	detail, ok := component.FailureDetail(err)
	require.True(t, ok)
	require.Equal(t, "live/dev/terragrunt.hcl", detail.Location.File)
	require.Equal(t, uint32(4), detail.Location.Line)
	require.Equal(t, uint32(9), detail.Location.Column)
	require.Equal(t, "vpc", detail.Unit)
	require.Equal(t, "live/dev", detail.Stack)
	require.Equal(t, "discover", detail.Operation)
	require.True(t, detail.UserSafe)
	require.Equal(t, []string{"fix it"}, detail.Hints)
}

func TestCancellationAndOutsideFilesAreNotRewritten(t *testing.T) {
	require.Equal(t, codes.Canceled, status.Code(rpcError(context.Canceled)))
	require.Equal(t, codes.DeadlineExceeded, status.Code(rpcError(context.DeadlineExceeded)))
	require.Equal(t, filepath.Join(string(filepath.Separator), "elsewhere", "x.hcl"), projectRelative(t.TempDir(), filepath.Join(string(filepath.Separator), "elsewhere", "x.hcl")))
	require.Equal(t, "atmos.yaml", projectRelative(t.TempDir(), "atmos.yaml"))
	plain := errors.New("plain")
	require.Equal(t, plain, scoped(plain, "prepare", "", nil))
}

func TestSettingsNameTheOffendingKey(t *testing.T) {
	for name, test := range map[string]struct {
		config map[string]any
		key    string
	}{
		"misspelled key": {map[string]any{"discoverey": map[string]any{}}, "components.terraform.terragrunt.discoverey"},
		"mode":           {map[string]any{"mode": "nativ"}, "components.terraform.terragrunt.mode"},
	} {
		t.Run(name, func(t *testing.T) {
			value, err := encode(test.config)
			require.NoError(t, err)
			err = settings(value)
			var problem *model.Problem
			require.ErrorAs(t, err, &problem)
			require.Equal(t, test.key, problem.Key)
			require.NotEmpty(t, problem.Hints)
			detail, ok := component.FailureDetail(rpcError(err))
			require.True(t, ok)
			require.Equal(t, test.key, detail.ConfigKey)
		})
	}
}

func TestUnitDiagnosticNamesTheUnit(t *testing.T) {
	root := t.TempDir()
	diagnostic := unitDiagnostic(root, model.UnitFailure{Stack: "live/dev", Name: "vpc", Err: &model.Problem{Code: model.CodeDependency, Summary: "Missing", Safe: true}})
	require.Equal(t, "vpc", diagnostic.Detail.Unit)
	require.Equal(t, "live/dev", diagnostic.Detail.Stack)
	require.Equal(t, "discover", diagnostic.Detail.Operation)
	require.Equal(t, pluginv1.Severity_SEVERITY_ERROR, diagnostic.Detail.Severity)
	require.Equal(t, "Missing", diagnostic.Message)
}

func TestDiscoveryReportsAFailedUnitAndKeepsTheHealthyOnes(t *testing.T) {
	req := componentRequest(t)
	healthy := model.DiscoveredUnit{Stack: "live", Name: "ok", Config: model.Resolution{Path: filepath.Join(req.RootDirectory, "live", "ok", "terragrunt.hcl"), Folders: []string{filepath.Join(req.RootDirectory, "module")}}}
	p := New(func(context.Context, model.Request) (*model.Resolution, error) {
		return &model.Resolution{
			Inventory: []model.DiscoveredUnit{healthy},
			Failures:  []model.UnitFailure{{Stack: "live", Name: "broken", Err: &model.Problem{Code: model.CodeSyntaxError, Summary: "Invalid", Safe: true}}},
		}, nil
	})
	response, err := p.Discover(t.Context(), &pluginv1.DiscoverComponentsRequest{RootDirectory: req.RootDirectory, Provider: req.Provider, ProviderConfiguration: req.ProviderConfiguration, Operation: req.Operation, IncludePaths: []string{"live/**"}})
	require.NoError(t, err)
	require.Len(t, response.Components, 1)
	require.Equal(t, "ok", response.Components[0].Address.Component)
	require.Len(t, response.Diagnostics, 1)
	require.Equal(t, "broken", response.Diagnostics[0].Detail.Unit)
}

// Exercise the evaluator and both provider output paths with an environment-derived target.
func TestDependencyDiagnosticsKeepEvaluatedTargetsHidden(t *testing.T) {
	for _, block := range []string{
		"dependency \"upstream\" { config_path = get_env(\"DEPENDENCY_TARGET\") }",
		"dependencies { paths = [get_env(\"DEPENDENCY_TARGET\")] }",
	} {
		t.Run(block, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			file := filepath.Join(root, "terragrunt.hcl")
			require.NoError(t, os.WriteFile(file, []byte(block), 0o600))
			const secret = "secret-dependency-target"
			_, err = engine.Resolve(t.Context(), model.Request{
				Path: file, BasePath: root, Phase: "discover", Command: "plan", Tofu: "must-not-execute",
				Environment: map[string]string{"DEPENDENCY_TARGET": "./" + secret},
			})
			var problem *model.Problem
			require.ErrorAs(t, err, &problem)
			require.Equal(t, model.CodeDependency, problem.Code)
			require.ErrorIs(t, err, os.ErrNotExist)
			require.Contains(t, problem.Err.Error(), secret, "the cause remains available locally")
			rpc := scoped(rpcError(err), "discover", root, &pluginv1.ComponentAddress{Stack: "live", Component: "unit"})
			require.NotContains(t, rpc.Error(), secret)
			wire, marshalErr := protojson.Marshal(status.Convert(rpc).Proto())
			require.NoError(t, marshalErr)
			require.NotContains(t, string(wire), secret)
			diagnostic := unitDiagnostic(root, model.UnitFailure{Stack: "live", Name: "unit", Err: err})
			wire, marshalErr = protojson.Marshal(diagnostic)
			require.NoError(t, marshalErr)
			require.NotContains(t, string(wire), secret)
			require.True(t, diagnostic.Detail.UserSafe)
			require.Equal(t, "terragrunt.hcl", diagnostic.Detail.Location.File)
			require.Equal(t, "dependency", diagnostic.Detail.ConfigKey)
			require.Equal(t, "unit", diagnostic.Detail.Unit)
			require.NotEmpty(t, diagnostic.Detail.Hints)
		})
	}
}
