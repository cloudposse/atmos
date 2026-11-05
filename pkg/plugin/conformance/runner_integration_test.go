package conformance

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	pluginbuild "github.com/cloudposse/atmos/pkg/plugin/build"
	"github.com/cloudposse/atmos/pkg/plugin/scaffold"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunnerChecksCommandOutputAndExplicitIntegration(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	require.True(t, ok)
	sdk := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "..", "sdk"))
	directory := filepath.Join(t.TempDir(), "fixture")
	require.NoError(t, scaffold.Create(t.Context(), directory, scaffold.Options{Name: "example/conformance", Module: "example.com/conformance", SDKPath: sdk, Runtime: "go", Capability: "auth.provider", Defaults: true}))
	require.NoError(t, pluginbuild.Run(t.Context(), directory, pluginbuild.WithStreams(process.Streams{Stdout: io.Discard, Stderr: io.Discard})))
	// The same generated command must run through project selection and report
	// its activation before producing output on the caller's data channel.
	atmos := &schema.AtmosConfiguration{BasePathConfigDir: t.TempDir()}
	atmos.Plugins.Packages = map[string]schema.PluginPackage{"local-auth": {Path: directory, Command: "about"}}
	var commandOutput, commandErrors bytes.Buffer
	var started []string
	require.NoError(t, bootstrap.Run(t.Context(), atmos, "local-auth", nil, bootstrap.Output{
		Stdout: &commandOutput, Stderr: &commandErrors,
		OnStart: func(alias, name, description string) {
			started = append(started, alias, name)
			require.NotEmpty(t, description)
			require.Empty(t, commandOutput.String(), "activation notification precedes command output")
		},
	}))
	require.Equal(t, []string{"local-auth", "about"}, started)
	fixtures, err := load(directory)
	require.NoError(t, err)
	require.Equal(t, fixtures.Commands[0].Stdout, commandOutput.String())
	require.Empty(t, commandErrors.String())
	s, err := prepare(directory, nil)
	require.NoError(t, err)
	require.ErrorIs(t, s.command(t.Context(), &CommandFixture{Name: "about", Stdout: "incorrect"}), errUtils.ErrTestsFailed)
	// An RPC failure is never accepted as an expected exit status.
	require.ErrorIs(t, s.command(t.Context(), &CommandFixture{Name: "about", Arguments: []string{"invalid"}, ExitCode: 1}), errUtils.ErrPluginProtocol)
	var progress []Result
	report, err := Run(t.Context(), directory, Options{Run: "about", Stdout: io.Discard, Stderr: io.Discard, OnResult: func(result Result) {
		progress = append(progress, result)
	}})
	require.NoError(t, err)
	assert.Equal(t, []Result{{Name: "manifest-handler-agreement", Passed: true}, {Name: "command/about", Passed: true}}, report.Results)
	assert.Equal(t, report.Results, progress, "each completed check is reported exactly once")
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	assert.JSONEq(t, `{"mode":"mock","results":[{"name":"manifest-handler-agreement","passed":true},{"name":"command/about","passed":true}]}`, string(raw), "live progress must not change machine output")
	// The default local suite must exercise the entire OIDC lifecycle, including
	// durable reuse across process restarts, refresh, revocation, and cleanup.
	report, err = Run(t.Context(), directory, Options{Stdout: io.Discard, Stderr: io.Discard})
	require.NoError(t, err, "%+v", report)
	assert.Equal(t, []Result{
		{Name: "manifest-handler-agreement", Passed: true},
		{Name: "oidc-lifecycle", Passed: true},
		{Name: "command/about", Passed: true},
	}, report.Results)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte("version: 1\nintegration:\n  - name: live-check\n    type: unsupported-fixture-task\n"), 0o600))
	progress = nil
	report, err = Run(t.Context(), directory, Options{Integration: true, Stdout: io.Discard, Stderr: io.Discard, OnResult: func(result Result) {
		progress = append(progress, result)
	}})
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	assert.Equal(t, "integration", report.Mode)
	assert.Equal(t, []Result{
		{Name: "manifest-handler-agreement", Passed: true},
		{Name: "authored-tests", Passed: false, Detail: "invalid workflow control step: unknown test step type \"unsupported-fixture-task\""},
	}, report.Results)
	assert.Equal(t, report.Results, progress, "failed checks must reach the live reporter too")
}
