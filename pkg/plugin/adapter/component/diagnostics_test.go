package component

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
