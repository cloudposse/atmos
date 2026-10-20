package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// deniedHostRule denies every source whose host is localhost.
const deniedHostRule = "input.host == 'localhost'"

// writePolicyDir writes a single policy manifest to a fresh directory and points ATMOS_POLICY_DIR at it.
func writePolicyDir(t *testing.T, manifest string) {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	t.Setenv("ATMOS_POLICY_DIR", directory)
}

// enableSourcePolicy activates admission with one source rule that denies localhost sources.
func enableSourcePolicy(t *testing.T) {
	t.Helper()
	writePolicyDir(t, fmt.Sprintf("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: downloads}\nspec:\n  policies:\n    deny:\n      on: source\n      action: deny\n      rule: %q\n", deniedHostRule))
}

// enableNonSourcePolicy activates admission with a rule for a different capability so sources are unguarded.
func enableNonSourcePolicy(t *testing.T) {
	t.Helper()
	writePolicyDir(t, "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: functions}\nspec:\n  policies:\n    deny:\n      on: function\n      action: deny\n      rule: \"input.name == 'exec'\"\n")
}

// enableInvalidPolicy points ATMOS_POLICY_DIR at an unparsable manifest so capability lookups fail.
func enableInvalidPolicy(t *testing.T) {
	t.Helper()
	writePolicyDir(t, "apiVersion: atmos/v1\nkind: Policy\nspec: [unterminated\n")
}

// restoreAdmissionHooks reinstalls the guard wiring found when it is called, after a test replaced it
// with fakes. The package tests cannot import admission or policy, which reach this package through
// pkg/ci and pkg/git, so the wiring comes from the external test package that loads admission.
func restoreAdmissionHooks(t *testing.T) {
	t.Helper()
	t.Cleanup(guard.SaveHooks())
}

// recordingGetter is a go-getter Getter that records which methods were invoked.
type recordingGetter struct {
	calls     []string
	err       error
	mode      getter.ClientMode
	gotDest   string
	gotSource string
}

func (r *recordingGetter) Get(dest string, u *url.URL) error {
	r.calls = append(r.calls, "Get")
	r.gotDest, r.gotSource = dest, u.String()
	return r.err
}

func (r *recordingGetter) GetFile(dest string, u *url.URL) error {
	r.calls = append(r.calls, "GetFile")
	r.gotDest, r.gotSource = dest, u.String()
	return r.err
}

func (r *recordingGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	r.calls = append(r.calls, "ClientMode")
	r.gotSource = u.String()
	return r.mode, r.err
}

func (r *recordingGetter) SetClient(*getter.Client) {}

func TestPolicyGetter_Methods(t *testing.T) {
	nativeErr := errors.New("native getter failure")
	const (
		allowedURL = "https://example.com/stack.yaml"
		deniedURL  = "https://localhost/stack.yaml"
	)
	invoke := map[string]func(g *policyGetter, u *url.URL) error{
		"Get":     func(g *policyGetter, u *url.URL) error { return g.Get("dest", u) },
		"GetFile": func(g *policyGetter, u *url.URL) error { return g.GetFile("dest", u) },
		"ClientMode": func(g *policyGetter, u *url.URL) error {
			_, err := g.ClientMode(u)
			return err
		},
	}

	for method, call := range invoke {
		t.Run(method+" delegates when allowed", func(t *testing.T) {
			enableSourcePolicy(t)
			native := &recordingGetter{}
			g := &policyGetter{Getter: native, ctx: context.Background(), scheme: "https"}
			u, err := url.Parse(allowedURL)
			require.NoError(t, err)

			require.NoError(t, call(g, u))
			assert.Equal(t, []string{method}, native.calls)
			assert.Equal(t, allowedURL, native.gotSource)
		})

		t.Run(method+" propagates native errors", func(t *testing.T) {
			enableSourcePolicy(t)
			native := &recordingGetter{err: nativeErr}
			g := &policyGetter{Getter: native, ctx: context.Background(), scheme: "https"}
			u, err := url.Parse(allowedURL)
			require.NoError(t, err)

			require.ErrorIs(t, call(g, u), nativeErr)
			assert.Equal(t, []string{method}, native.calls)
		})

		t.Run(method+" denied never reaches the native getter", func(t *testing.T) {
			enableSourcePolicy(t)
			native := &recordingGetter{}
			g := &policyGetter{Getter: native, ctx: context.Background(), scheme: "https"}
			u, err := url.Parse(deniedURL)
			require.NoError(t, err)

			require.ErrorIs(t, call(g, u), errUtils.ErrPolicyDenied)
			assert.Empty(t, native.calls)
		})

		t.Run(method+" rejects mercurial even when policy allows", func(t *testing.T) {
			enableSourcePolicy(t)
			native := &recordingGetter{}
			g := &policyGetter{Getter: native, ctx: context.Background(), scheme: "hg"}
			u, err := url.Parse(allowedURL)
			require.NoError(t, err)

			require.ErrorIs(t, call(g, u), errUtils.ErrPolicyInputUnavailable)
			assert.Empty(t, native.calls)
		})
	}
}

func TestPolicyGetter_ClientModeReturnsNativeMode(t *testing.T) {
	enableSourcePolicy(t)
	native := &recordingGetter{mode: getter.ClientModeDir}
	g := &policyGetter{Getter: native, ctx: context.Background(), scheme: "https"}
	u, err := url.Parse("https://example.com/repo")
	require.NoError(t, err)

	mode, err := g.ClientMode(u)
	require.NoError(t, err)
	assert.Equal(t, getter.ClientModeDir, mode)
}

func TestPolicyGetter_ClientModeDeniedReturnsAnyMode(t *testing.T) {
	enableSourcePolicy(t)
	native := &recordingGetter{mode: getter.ClientModeDir}
	g := &policyGetter{Getter: native, ctx: context.Background(), scheme: "https"}
	u, err := url.Parse("https://localhost/repo")
	require.NoError(t, err)

	mode, err := g.ClientMode(u)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	assert.Equal(t, getter.ClientModeAny, mode)
	assert.Empty(t, native.calls)
}
