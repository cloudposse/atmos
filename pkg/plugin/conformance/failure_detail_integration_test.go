package conformance

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginbuild "github.com/cloudposse/atmos/pkg/plugin/build"
	"github.com/cloudposse/atmos/pkg/plugin/scaffold"
	"github.com/cloudposse/atmos/pkg/process"
)

// buildScaffold compiles a scaffolded plugin once so each case can vary only plugin.tests.yaml.
func buildScaffold(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	require.True(t, ok)
	sdk := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "..", "sdk"))
	directory := filepath.Join(t.TempDir(), "fixture")
	require.NoError(t, scaffold.Create(t.Context(), directory, scaffold.Options{Name: "example/conformance", Module: "example.com/conformance", SDKPath: sdk, Runtime: "go", Capability: "auth.provider", Defaults: true}))
	require.NoError(t, pluginbuild.Run(t.Context(), directory, pluginbuild.WithStreams(process.Streams{Stdout: io.Discard, Stderr: io.Discard})))
	return directory
}

func writeTests(t *testing.T, directory, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte(body), 0o600))
}

func TestFailingFixturesReportExpectedVersusActualAndNoMatchListsTests(t *testing.T) {
	directory := buildScaffold(t)

	t.Run("wrong stdout and exit code are named with both values", func(t *testing.T) {
		writeTests(t, directory, "version: 1\ncommands:\n  - name: about\n    stdout: \"WRONG EXPECTATION\\n\"\n    stderr: \"\"\n    exit_code: 4\n")
		report, err := Run(t.Context(), directory, Options{Stdout: io.Discard, Stderr: io.Discard})
		require.ErrorIs(t, err, errUtils.ErrTestsFailed)
		require.Len(t, report.Results, 2)
		failed := report.Results[1]
		assert.Equal(t, "command/about", failed.Name)
		assert.False(t, failed.Passed)
		assert.Contains(t, failed.Detail, "stdout does not match the fixture")
		assert.Contains(t, failed.Detail, "WRONG EXPECTATION")
		assert.Contains(t, failed.Detail, "No authentication was performed", "the actual output is shown")
		assert.Contains(t, failed.Detail, "exit_code does not match the fixture: expected 4, actual 0")
		assert.NotContains(t, failed.Detail, "stderr does not match", "fields that agree are not reported")
		raw, marshalErr := json.Marshal(report)
		require.NoError(t, marshalErr)
		assert.Contains(t, string(raw), `"detail":"stdout does not match`)
	})

	t.Run("a filter that selects nothing lists the available tests", func(t *testing.T) {
		writeTests(t, directory, "version: 1\ncommands:\n  - name: about\n    stdout: \"\"\n    stderr: \"\"\n    exit_code: 0\nsteps: []\n")
		_, err := Run(t.Context(), directory, Options{Run: "nothing-like-this", Stdout: io.Discard, Stderr: io.Discard})
		require.ErrorIs(t, err, errUtils.ErrPluginTestsNotMatched)
		require.NotErrorIs(t, err, errUtils.ErrTestsFailed)
		hints := errUtils.AllHints(err)
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0], "command/about")
	})

	t.Run("a missing executable is a failed handshake with a build hint", func(t *testing.T) {
		missing := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(missing, "plugin.yaml"), []byte(conformanceManifest), 0o600))
		writeTests(t, missing, "version: 1\n")
		report, err := Run(t.Context(), missing, Options{})
		require.ErrorIs(t, err, errUtils.ErrTestsFailed)
		require.Len(t, report.Results, 1)
		assert.Contains(t, report.Results[0].Detail, "atmos plugin build")
	})
}

// Each broken fixture models a plugin regression the conformance suite must
// reject. The real generated plugin still performs its preceding lifecycle steps.
func TestOIDCConformanceRejectsBrokenLifecycleBehavior(t *testing.T) {
	directory := buildScaffold(t)
	cases := []struct{ name, file, original, broken, detail string }{
		{"interactive login", "provider.go", "current, err = p.login(ctx, config, issuer, oauth)", "current, err = nil, unavailable()", "interactive PKCE login failed"},
		{"cached login", "provider.go", "if current != nil && current.Token.Valid() { return credentialResult(config, current) }", "if current != nil && current.Token.Valid() { return nil, unavailable() }", "cached login failed"},
		{"changed cached token", "provider.go", "if current != nil && current.Token.Valid() { return credentialResult(config, current) }", "if current != nil && current.Token.Valid() { current.Token.AccessToken += \"-changed\"; return credentialResult(config, current) }", "a cached session must return the same token"},
		{"broker authentication", "broker.go", "input.Header.Set(\"Authorization\", \"Bearer \" + accessToken)", "input.Header.Set(\"Authorization\", \"Bearer invalid-\" + accessToken)", "identity credential exchange failed"},
		{"broker durable cache", "broker.go", "b.cachedExchange(ctx, request)", "b.exchange(ctx, request)", "broker must reuse its cached credentials"},
		{"expired login", "provider.go", "updated.Token = token", "updated.Token = current.Token", "login with an expired session failed"},
		{"logout failure", "logout.go", "if response.StatusCode < 200 || response.StatusCode >= 300", "if response.StatusCode > 0", "logout failed"},
		{"logout retains session", "logout.go", "p.api.Delete(ctx, \"session/v1\")", "p.api.Delete(ctx, \"nonexistent-session\")", "login after logout must fail"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(directory, tc.file)
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(original), tc.original)
			t.Cleanup(func() { require.NoError(t, os.WriteFile(path, original, 0o600)) })
			changed := strings.ReplaceAll(string(original), tc.original, tc.broken)
			require.NoError(t, os.WriteFile(path, []byte(changed), 0o600))
			var diagnostics strings.Builder
			require.NoError(t, pluginbuild.Run(t.Context(), directory, pluginbuild.WithStreams(process.Streams{Stdout: io.Discard, Stderr: &diagnostics})), diagnostics.String())
			report, err := Run(t.Context(), directory, Options{Run: "oidc-lifecycle", Stdout: io.Discard, Stderr: io.Discard})
			require.ErrorIs(t, err, errUtils.ErrTestsFailed)
			require.Len(t, report.Results, 2)
			require.True(t, report.Results[0].Passed, "the altered fixture must still satisfy its declared wire contract")
			require.Equal(t, "oidc-lifecycle", report.Results[1].Name)
			require.False(t, report.Results[1].Passed)
			require.Contains(t, report.Results[1].Detail, tc.detail)
		})
	}
}
