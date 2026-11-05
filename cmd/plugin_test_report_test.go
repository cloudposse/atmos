package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/plugin/reported"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// capturePluginUI routes the UI channel into a buffer for the duration of the test.
func capturePluginUI(t *testing.T) *bytes.Buffer {
	t.Helper()
	ioContext, err := atmosio.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioContext)
	t.Cleanup(ui.Reset)
	var output bytes.Buffer
	t.Cleanup(atmosio.PushUIWriter(&output))
	return &output
}

// builtPluginProject scaffolds and compiles a plugin inside a plugin test project.
func builtPluginProject(t *testing.T) (*schema.AtmosConfiguration, string) {
	t.Helper()
	repository, err := filepath.Abs("..")
	require.NoError(t, err)
	config := pluginCoverageProject(t)
	directory := filepath.Join(config.BasePath, "plugin")
	_, err = invokePluginCoverage(t, config, "create", directory, "--defaults", "--sdk-path", filepath.Join(repository, "sdk"))
	require.NoError(t, err)
	_, err = invokePluginCoverage(t, config, "build", directory)
	require.NoError(t, err)
	return config, directory
}

func writePluginFixture(t *testing.T, directory, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte(body), 0o600))
}

const wrongStdoutFixture = "version: 1\ncommands:\n  - name: about\n    stdout: \"WRONG EXPECTATION\\n\"\n    stderr: \"\"\n    exit_code: 0\n"

func TestPluginTestShowsFailureEvidenceOnceInTextAndJSON(t *testing.T) {
	config, directory := builtPluginProject(t)
	writePluginFixture(t, directory, wrongStdoutFixture)
	output := capturePluginUI(t)

	_, err := invokePluginCoverage(t, config, "test", directory)
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	assert.True(t, reported.IsReported(err), "the summary already shows the failure; the root handler must not print it again")
	text := ansi.Strip(output.String())
	assert.Contains(t, text, "stdout does not match the fixture")
	assert.Contains(t, text, "WRONG EXPECTATION")
	assert.Contains(t, text, "No authentication was performed", "the actual output is shown under the failing check")
	assert.Equal(t, 1, strings.Count(text, "Plugin tests failed"), text)

	stdout, err := invokePluginCoverage(t, config, "test", directory, "--format=json")
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	assert.True(t, reported.IsReported(err))
	var report struct {
		Results []struct {
			Name   string `json:"name"`
			Passed bool   `json:"passed"`
			Detail string `json:"detail"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &report), stdout)
	require.NotEmpty(t, report.Results)
	last := report.Results[len(report.Results)-1]
	assert.Equal(t, "command/about", last.Name)
	assert.False(t, last.Passed)
	assert.Contains(t, last.Detail, "stdout does not match the fixture")
}

func TestPluginTestFilterThatMatchesNothingListsAvailableTests(t *testing.T) {
	config, directory := builtPluginProject(t)
	writePluginFixture(t, directory, wrongStdoutFixture)
	output := capturePluginUI(t)

	_, err := invokePluginCoverage(t, config, "test", directory, "--run=no-such-test")
	require.ErrorIs(t, err, errUtils.ErrPluginTestsNotMatched)
	require.NotErrorIs(t, err, errUtils.ErrTestsFailed)
	assert.False(t, reported.IsReported(err), "an unmatched filter has no report, so the root handler must explain it")
	rendered := formatPluginError(err)
	assert.Contains(t, rendered, "no-such-test")
	assert.Contains(t, rendered, "command/about")
	assert.NotContains(t, ansi.Strip(output.String()), "Plugin tests failed")
}

func TestPluginTestRejectsUnsupportedFormatThroughSharedValidation(t *testing.T) {
	config := pluginCoverageProject(t)
	_, err := invokePluginCoverage(t, config, "test", t.TempDir(), "--format=xml")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Contains(t, err.Error(), "text, json")
}

func TestPluginAuthoringOperationsAreGovernedByPolicyNotSignatureRequirements(t *testing.T) {
	config, directory := builtPluginProject(t)
	writePluginFixture(t, directory, "version: 1\ncommands:\n  - name: about\n    stdout: \"\"\n    stderr: \"\"\n    exit_code: 0\n")
	// An unsigned local plugin must remain buildable and testable under the strictest consumer settings.
	config.Plugins.Policy.RequireSignature = true
	capturePluginUI(t)

	_, err := invokePluginCoverage(t, config, "build", directory)
	require.NoError(t, err)
	// The fixture above intentionally mismatches stdout; the point is that it ran at all.
	_, err = invokePluginCoverage(t, config, "test", directory)
	require.ErrorIs(t, err, errUtils.ErrTestsFailed, "tests ran and reported a fixture mismatch rather than a signature or policy refusal")
	require.NotErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)

	// `on: plugin` rules are the only gate for `test`; `build` is not a policy operation, so even a
	// broad company-registry rule that rejects every development origin does not block compiling.
	t.Cleanup(ci.SwapRegistryForTest())
	policies := filepath.Join(config.BasePath, "policy.d")
	require.NoError(t, os.Mkdir(policies, 0o700))
	t.Setenv("ATMOS_POLICY_DIR", policies)
	const document = `apiVersion: atmos/v1
kind: Policy
metadata: {name: company}
spec:
  policies:
    no-development:
      on: plugin
      action: deny
      rule: input.origin == 'development'
      reason: Plugins must use the approved company registry.
`
	require.NoError(t, os.WriteFile(filepath.Join(policies, "plugins.yaml"), []byte(document), 0o600))
	_, err = invokePluginCoverage(t, config, "build", directory)
	require.NoError(t, err, "plugin build must not be denied by plugin admission rules")
	_, err = invokePluginCoverage(t, config, "test", directory)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	assert.Contains(t, formatPluginError(err), "Plugins must use the approved company registry.")
}

func TestPluginIntegrationRequiresAuthoredStepsWithoutProjectConfig(t *testing.T) {
	_, directory := pluginNativeProject(t)
	writePluginFixture(t, directory, "version: 1\n")
	output := capturePluginUI(t)

	_, err := invokePluginCoverage(t, nil, "test", directory, "--integration")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.Contains(t, formatPluginError(err), "no integration entries")
	require.Contains(t, ansi.Strip(output.String()), "live integration steps")
}
