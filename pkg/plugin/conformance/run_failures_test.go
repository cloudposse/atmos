package conformance

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/testkit"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunRejectsMissingAndInvalidPackageInputs(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	report, err := Run(t.Context(), directory, Options{})
	require.Error(t, err)
	assert.Nil(t, report)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte("version: 1\n"), 0o600))
	report, err = Run(t.Context(), directory, Options{})
	require.Error(t, err)
	assert.Nil(t, report)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(conformanceManifest), 0o600))
	report, err = Run(t.Context(), directory, Options{Integration: true})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Equal(t, "integration", report.Mode)
	assert.Empty(t, report.Results)
	report, err = Run(t.Context(), directory, Options{})
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	require.Len(t, report.Results, 1, "a failed handshake is the first failing check, not a silent abort")
	assert.Equal(t, "manifest-handler-agreement", report.Results[0].Name)
	assert.False(t, report.Results[0].Passed)
	assert.Contains(t, report.Results[0].Detail, "atmos plugin build")
}

const conformanceManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata: {name: example/conformance, version: 1.0.0}
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  capabilities:
    commands:
      about: {description: Test command}
`

func TestFixturesReportUndeclaredAndUnstartableExports(t *testing.T) {
	t.Parallel()
	registry := extension.New()
	require.NoError(t, registry.Register(extension.Descriptor{Capability: "command", Path: []string{"commands"}, Version: 1, Validate: func(extension.Binding) error { return nil }}))
	s := suite{registry: registry, executable: filepath.Join(t.TempDir(), "missing")}
	require.ErrorIs(t, s.command(t.Context(), &CommandFixture{Name: "absent"}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, s.oidc(t.Context(), &OIDCFixture{Provider: "absent", Identity: "absent"}), errUtils.ErrPluginInvalidConfig)
	s.bindings = []extension.Binding{{Capability: "command", Name: "about"}}
	require.ErrorIs(t, s.command(t.Context(), &CommandFixture{Name: "about"}), errUtils.ErrPluginProtocol)
	report := &Report{}
	require.ErrorIs(t, s.runFixtures(t.Context(), &Config{Commands: []CommandFixture{{Name: "about"}}}, &Options{}, report), errUtils.ErrTestsFailed)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "command/about", report.Results[0].Name)
	assert.False(t, report.Results[0].Passed)
	assert.Contains(t, report.Results[0].Detail, "is not available")
	report = &Report{}
	require.ErrorIs(t, s.runFixtures(t.Context(), &Config{OIDC: &OIDCFixture{Provider: "missing"}}, &Options{}, report), errUtils.ErrTestsFailed)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "oidc-lifecycle", report.Results[0].Name)
	assert.False(t, report.Results[0].Passed)
	assert.Contains(t, report.Results[0].Detail, "must export auth.provider")
}

func TestAuthoredStepsFilterAndReportFailures(t *testing.T) {
	t.Parallel()
	report := &Report{}
	require.ErrorIs(t, runSteps(t.Context(), t.TempDir(), nil, &Options{Integration: true}, report), errUtils.ErrPluginTestsNotMatched)
	assert.Empty(t, report.Results)
	steps := []schema.WorkflowStep{{Name: "check", Type: "unsupported-fixture-task"}}
	require.Error(t, runSteps(t.Context(), t.TempDir(), steps, &Options{Stdout: io.Discard, Stderr: io.Discard}, report))
	require.Len(t, report.Results, 1)
	assert.Equal(t, "authored-tests", report.Results[0].Name)
	assert.False(t, report.Results[0].Passed)
	assert.Contains(t, report.Results[0].Detail, "unsupported-fixture-task")
}

func TestAuthoredConformanceReportsPassingAndFailingAssertions(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		passed       bool
	}{
		{"passing", "output = 2 + 2", true},
		{"failing", `fail("expected four widgets")`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := suite{registry: extension.New(), directory: t.TempDir()}
			config := &Config{Steps: []schema.WorkflowStep{{Name: "widget-check", Type: "script", Interpreter: "starlark", Script: tc.script}}}
			report := &Report{}
			err := s.runFixtures(t.Context(), config, &Options{Stdout: io.Discard, Stderr: io.Discard}, report)
			if tc.passed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Len(t, report.Results, 1)
			require.Equal(t, "authored-tests", report.Results[0].Name)
			require.Equal(t, tc.passed, report.Results[0].Passed)
			if tc.passed {
				require.Empty(t, report.Results[0].Detail)
			} else {
				require.Contains(t, report.Results[0].Detail, "per-test output was written to stderr")
			}
		})
	}
}

func TestUnmatchedConformanceSelectionListsOIDCAndAuthoredSteps(t *testing.T) {
	t.Parallel()
	s := suite{registry: extension.New()}
	config := &Config{OIDC: &OIDCFixture{Provider: "test/provider", Identity: "test/identity"}, Steps: []schema.WorkflowStep{{Name: "widget-check"}}}
	err := s.runFixtures(t.Context(), config, &Options{Run: "absent"}, &Report{})
	require.ErrorIs(t, err, errUtils.ErrPluginTestsNotMatched)
	hints := errUtils.AllHints(err)
	require.Len(t, hints, 1)
	require.Contains(t, hints[0], "oidc-lifecycle")
	require.Contains(t, hints[0], "widget-check")
}

func TestOIDCConformanceDisallowsExternalHTTPAndPropagatesStartFailure(t *testing.T) {
	t.Parallel()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://issuer.example", nil)
	require.NoError(t, err)
	response, err := (loopbackTransport{}).RoundTrip(request)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.ErrorIs(t, err, errUtils.ErrPluginAPI)
	run := oidcRun{suite: &suite{executable: filepath.Join(t.TempDir(), "missing")}, fixture: &OIDCFixture{}}
	_, err = run.authenticate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrPluginProtocol)
	require.ErrorIs(t, run.broker(t.Context(), nil), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, run.exchange(t.Context(), nil, nil), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, run.logout(t.Context()), errUtils.ErrPluginProtocol)
	require.ErrorIs(t, run.lifecycle(t.Context()), errUtils.ErrTestsFailed)
	require.Error(t, run.openBrowser("https://issuer.example"))
}

func TestConformanceReportsInvalidExportsAndUnstartableBuild(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(conformanceManifest), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte("version: 1\n"), 0o600))
	_, err := Run(t.Context(), directory, Options{Registry: extension.New()})
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	require.NoError(t, os.Mkdir(filepath.Join(directory, "bin"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bin", "plugin"), []byte("not an executable"), 0o700))
	report, err := Run(t.Context(), directory, Options{})
	require.ErrorIs(t, err, errUtils.ErrTestsFailed)
	require.Len(t, report.Results, 1)
	require.False(t, report.Results[0].Passed)
	require.Contains(t, report.Results[0].Detail, "did not start")
}

func TestConformanceRejectsMissingExtensionAndIncompleteLifecycle(t *testing.T) {
	t.Parallel()
	s := suite{registry: extension.New(), bindings: []extension.Binding{{Capability: "unsupported", Name: "fixture"}}}
	report := &Report{}
	require.ErrorIs(t, s.extensions(t.Context(), "", report), errUtils.ErrPluginUnsupportedExtension)
	assert.Empty(t, report.Results)
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	issuer, err := testkit.NewIssuer()
	require.NoError(t, err)
	defer issuer.Close()
	run := oidcRun{storage: storage, issuer: issuer}
	require.ErrorIs(t, run.refreshAndLogout(t.Context(), nil), errUtils.ErrTestsFailed, "missing private session cannot count as verified refresh")
	require.ErrorIs(t, run.checkLoggedOut(), errUtils.ErrTestsFailed, "empty storage alone does not prove a token was revoked")
}

func TestOIDCFixtureRestrictsAuthorizationRedirectsToHTTPLoopback(t *testing.T) {
	t.Parallel()
	issuer, err := testkit.NewIssuer()
	require.NoError(t, err)
	t.Cleanup(issuer.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, tt := range []struct {
		name, callback string
		status         int
	}{
		{"loopback", "http://127.0.0.1:12345/callback?keep=value", http.StatusFound},
		{"remote host", "http://example.com/callback", http.StatusBadRequest},
		{"script scheme", "javascript://127.0.0.1/callback", http.StatusBadRequest},
		{"file transfer scheme", "ftp://127.0.0.1/callback", http.StatusBadRequest},
		{"https", "https://127.0.0.1/callback", http.StatusBadRequest},
		{"scheme relative", "//127.0.0.1/callback", http.StatusBadRequest},
		{"userinfo", "http://user:password@127.0.0.1/callback", http.StatusBadRequest},
		{"fragment", "http://127.0.0.1/callback#unexpected", http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			query := url.Values{
				"redirect_uri": {tt.callback}, "client_id": {"fixture-client"}, "state": {"expected-state"},
				"nonce": {"fixture-nonce"}, "code_challenge": {"fixture-challenge"}, "code_challenge_method": {"S256"},
			}
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, issuer.Server.URL+"/authorize?"+query.Encode(), nil)
			require.NoError(t, err)
			response, err := client.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, tt.status, response.StatusCode)
			if tt.status != http.StatusFound {
				require.Empty(t, response.Header.Get("Location"), "an unsafe callback must never be redirected")
				return
			}
			location, err := response.Location()
			require.NoError(t, err)
			require.Equal(t, "http", location.Scheme)
			require.Equal(t, "127.0.0.1:12345", location.Host)
			require.Equal(t, "/callback", location.Path)
			require.Equal(t, "expected-state", location.Query().Get("state"))
			require.Equal(t, "value", location.Query().Get("keep"))
			require.NotEmpty(t, location.Query().Get("code"))
		})
	}
}
