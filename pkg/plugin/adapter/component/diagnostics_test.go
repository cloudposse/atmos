package component

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func prepareWith(t *testing.T, command string) error {
	t.Helper()
	request := &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress(), Operation: &pluginv1.ComponentOperation{Command: command}}
	_, err := fixtureProvider(t).Validate(t.Context(), request)
	return err
}

func TestStructuredFailureRendersSafeDetail(t *testing.T) {
	err := prepareWith(t, "detail-safe")
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	assert.Equal(t, "Unsupported Terragrunt setting", err.Error())

	text := strings.Join(errUtils.AllDetails(err), "\n")
	for _, want := range []string{"Component `vpc` in stack `live/dev`", "live/dev/vpc/terragrunt.hcl:7:3", "setting `terraform_binary`", "during prepare", "terraform_binary is not implemented."} {
		assert.Contains(t, text, want)
	}
	assert.Equal(t, []string{"Remove terraform_binary."}, errUtils.AllHints(err))
	location, found := errUtils.GetLocation(err)
	require.True(t, found)
	assert.Equal(t, "live/dev/vpc/terragrunt.hcl", location.File)
	assert.Equal(t, 7, location.Line)
}

func TestStructuredFailureWithholdsUnsafeDetail(t *testing.T) {
	err := prepareWith(t, "detail-unsafe")
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	rendered := err.Error() + strings.Join(errUtils.AllDetails(err), "\n")
	assert.NotContains(t, rendered, "ghp_SECRET")
	hints := errUtils.AllHints(err)
	require.Len(t, hints, 2)
	assert.Equal(t, "Remove terraform_binary.", hints[0])
	assert.Contains(t, hints[1], "ATMOS_LOGS_LEVEL=Debug")
}

func TestErrorSeverityDiagnosticFailsAndWarningDoesNot(t *testing.T) {
	request := &pluginv1.ComponentRequest{RootDirectory: t.TempDir(), Address: fixtureAddress(), Operation: &pluginv1.ComponentOperation{Command: "diagnostic-error"}}
	_, err := fixtureProvider(t).Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Equal(t, "Bad setting", err.Error())

	warning := &pluginv1.Diagnostic{Detail: &pluginv1.ErrorDetail{Code: "skipped", Summary: "Skipped", Severity: pluginv1.Severity_SEVERITY_WARNING}}
	require.NoError(t, firstDiagnosticError([]*pluginv1.Diagnostic{warning}))
	require.ErrorIs(t, firstDiagnosticError([]*pluginv1.Diagnostic{warning, {Field: "x", Message: "legacy"}}), errUtils.ErrPluginInvalidConfig)
}

func TestMissingDetailFallsBackToSentinel(t *testing.T) {
	require.ErrorIs(t, DetailError(nil, 0), errUtils.ErrPluginAPI)
}

// A legacy diagnostic keeps its message as unsafe detail: never shown, but pointed at by the debug hint.
func TestLegacyDiagnosticKeepsItsMessageForDebugLogging(t *testing.T) {
	err := DiagnosticError(&pluginv1.Diagnostic{Field: "region", Message: "region ghp_LEGACY is invalid"})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	rendered := err.Error() + strings.Join(errUtils.AllDetails(err), "\n")
	assert.NotContains(t, rendered, "ghp_LEGACY")
	hints := errUtils.AllHints(err)
	require.NotEmpty(t, hints)
	assert.Contains(t, hints[len(hints)-1], "ATMOS_LOGS_LEVEL=Debug")
	var failure *Failure
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, "region ghp_LEGACY is invalid", failure.Detail.Detail)
	assert.False(t, failure.Detail.UserSafe)
}

// The attached location is masked before annotation escaping can hide a mask literal from the
// final masker.
func TestDiagnosticLocationIsMaskedBeforeItIsAttached(t *testing.T) {
	t.Cleanup(atmosio.Reset)
	atmosio.Reset()
	require.NoError(t, atmosio.Initialize())
	atmosio.RegisterSecret("tok,en-7d3a")
	err := DetailError(&pluginv1.ErrorDetail{Code: "bad", Summary: "Bad", Location: &pluginv1.SourceLocation{File: "live/tok,en-7d3a/terragrunt.hcl", Line: 3}}, 0)
	location, found := errUtils.GetLocation(err)
	require.True(t, found)
	assert.NotContains(t, location.File, "tok,en-7d3a")
	assert.Equal(t, 3, location.Line)
}

// An InvalidArgument failure that carries a structured detail stays on the structured path: the plugin's
// safe detail reaches the user and an unsafe detail does not. A bare InvalidArgument status still
// discloses nothing from its RPC message.
func TestInvalidArgumentStructuredDetailKeepsSafeContract(t *testing.T) {
	for _, safe := range []bool{true, false} {
		detail := &pluginv1.ErrorDetail{Code: "evaluation_failed", Summary: "Evaluation failed", Location: &pluginv1.SourceLocation{File: "live/app/terragrunt.hcl", Line: 2}, UserSafe: safe}
		if safe {
			detail.Detail = "locals.region is not defined."
		} else {
			detail.Detail = "token ghp_INVALIDARG leaked"
		}
		wire, err := status.New(codes.InvalidArgument, "secret RPC payload").WithDetails(detail)
		require.NoError(t, err)
		// Round-trip the status protobuf as the RPC transport does.
		data, err := proto.Marshal(wire.Proto())
		require.NoError(t, err)
		decoded := wire.Proto()
		require.NoError(t, proto.Unmarshal(data, decoded))

		result := invocationError(status.FromProto(decoded).Err())
		require.ErrorIs(t, result, errUtils.ErrPluginInvalidConfig)
		var failure *Failure
		require.ErrorAs(t, result, &failure, "structured detail must not degrade to a bare sentinel")
		rendered := result.Error() + strings.Join(errUtils.AllDetails(result), "\n")
		assert.Equal(t, "Evaluation failed", result.Error())
		assert.NotContains(t, rendered, "secret RPC payload")
		assert.Contains(t, rendered, "live/app/terragrunt.hcl:2")
		if safe {
			assert.Contains(t, rendered, "locals.region is not defined.")
		} else {
			assert.NotContains(t, rendered, "ghp_INVALIDARG")
		}
	}

	bare := invocationError(status.Error(codes.InvalidArgument, "secret RPC payload"))
	require.ErrorIs(t, bare, errUtils.ErrPluginAPI)
	assert.NotContains(t, bare.Error(), "secret")
}
