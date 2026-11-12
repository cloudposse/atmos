package starlark

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

type browserOpenFunc func(string) error

func (f browserOpenFunc) Open(target string) error { return f(target) }

func TestBrowserOpen(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`output = browser.open("http://localhost:3000/path?q=a%20b#docs") == None`,
		`output = browser.open(url="https://example.com/path?q=a%20b#docs") == None`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			var opened string
			result, err := runSource(t, source, WithBrowserOpener(browserOpenFunc(func(target string) error {
				opened = target
				return nil
			})))
			require.NoError(t, err)
			assert.Equal(t, "true", result.Value)
			assert.Contains(t, source, opened)
		})
	}
}

func TestBrowserOpenRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	for _, argument := range []string{
		`""`, `"/tmp/page.html"`, `"file:///tmp/page.html"`,
		`"javascript:alert(1)"`, `"https://"`, `"https://user:secret@example.com"`,
		`"https://example.com\n"`, `"https://%zz"`, `"https://:3000"`,
		`"--help"`, `None`, `42`, ``, `url="https://example.com", unknown=True`,
	} {
		t.Run(argument, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, "browser.open("+argument+")", WithBrowserOpener(browserOpenFunc(func(string) error {
				t.Fatal("invalid arguments must not reach the browser")
				return nil
			})))
			require.Error(t, err)
		})
	}
}

func TestBrowserOpenFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("desktop unavailable")
	_, err := runSource(t, `browser.open("https://example.com")`,
		WithBrowserOpener(browserOpenFunc(func(string) error { return cause })))
	require.ErrorIs(t, err, cause)
	assert.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
	assert.ErrorContains(t, err, "could not launch the default browser")
}

func TestBrowserOpenDryRunAndCancellation(t *testing.T) {
	t.Parallel()
	engine := New(WithBrowserOpener(browserOpenFunc(func(string) error {
		t.Fatal("dry runs and canceled executions must not launch a browser")
		return nil
	})))
	_, err := engine.Execute(t.Context(), script.Spec{Source: `browser.open("https://example.com")`, DryRun: true})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = engine.Execute(ctx, script.Spec{Source: `browser.open("https://example.com")`})
	require.ErrorIs(t, err, context.Canceled)
}

func TestBrowserOpenFromModule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "portal.star"), []byte("def launch(url):\n    browser.open(url)\n"), 0o600))
	var opened string
	engine := New(WithBrowserOpener(browserOpenFunc(func(target string) error {
		opened = target
		return nil
	})))
	_, err := engine.Execute(t.Context(), script.Spec{
		WorkingDirectory: dir,
		Source:           fmt.Sprintf("portal = module.load(%q)\nportal.launch(%q)", "portal.star", "http://localhost:3000"),
	})
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000", opened)
}
