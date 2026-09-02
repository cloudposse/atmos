package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLicenseReportFiltersNoiseLines(t *testing.T) {
	raw := "W1234: could not resolve some/module\n" +
		"github.com/example/one,https://example.com/one/LICENSE,Apache-2.0\n" +
		"E5678: unknown license for another/module\n" +
		"github.com/example/two,https://example.com/two/LICENSE,MIT\n"

	entries := parseLicenseReport(raw)

	require.Len(t, entries, 2)
	assert.Equal(t, LicenseEntry{Module: "github.com/example/one", URL: "https://example.com/one/LICENSE", License: "Apache-2.0"}, entries[0])
	assert.Equal(t, LicenseEntry{Module: "github.com/example/two", URL: "https://example.com/two/LICENSE", License: "MIT"}, entries[1])
}

func TestParseLicenseReportSkipsBlankLines(t *testing.T) {
	raw := "\ngithub.com/example/one,https://example.com/one/LICENSE,Apache-2.0\n\n"

	entries := parseLicenseReport(raw)

	require.Len(t, entries, 1)
	assert.Equal(t, "github.com/example/one", entries[0].Module)
}

func TestParseLicenseReportEmptyInput(t *testing.T) {
	assert.Empty(t, parseLicenseReport(""))
}

// TestParseLicenseReportSkipsMalformedLines covers go-licenses' multi-line
// stderr warnings, whose continuation lines (a bare file path, no W/E
// prefix) don't parse as a 3-field CSV row and must be dropped rather than
// aborting the whole report -- this is what actually happens against this
// repository's real dependency set (asm/header files referenced by a
// license-detection warning land on their own line).
func TestParseLicenseReportSkipsMalformedLines(t *testing.T) {
	raw := "only,two\n" +
		"/Users/erik/go/pkg/mod/example.com/foo@v1.0.0/bar.s\n" +
		"github.com/example/one,https://example.com/one/LICENSE,Apache-2.0\n"

	entries := parseLicenseReport(raw)

	require.Len(t, entries, 1)
	assert.Equal(t, "github.com/example/one", entries[0].Module)
}

func TestEnvOrDefault(t *testing.T) {
	const key = "NOTICEGEN_TEST_ENV_OR_DEFAULT"
	t.Setenv(key, "")
	assert.Equal(t, "fallback", envOrDefault(key, "fallback"))

	t.Setenv(key, "custom")
	assert.Equal(t, "custom", envOrDefault(key, "fallback"))
}

func TestGoLicensesVersionDefault(t *testing.T) {
	t.Setenv("GO_LICENSES_VERSION", "")
	assert.Equal(t, defaultGoLicensesVersion, goLicensesVersion())

	t.Setenv("GO_LICENSES_VERSION", "v9.9.9")
	assert.Equal(t, "v9.9.9", goLicensesVersion())
}

func TestDefaultLicenseEnv(t *testing.T) {
	t.Setenv("LICENSE_GOOS", "")
	t.Setenv("LICENSE_GOARCH", "")
	t.Setenv("LICENSE_CGO_ENABLED", "")

	env := defaultLicenseEnv()

	assert.Equal(t, "linux", env.GOOS)
	assert.Equal(t, "amd64", env.GOARCH)
	assert.Equal(t, "1", env.CGOEnabled)
}

func TestGoEnv(t *testing.T) {
	out, err := goEnv("GOPATH")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}

func wantGoLicensesBinName() string {
	if runtime.GOOS == "windows" {
		return "go-licenses.exe"
	}
	return "go-licenses"
}

func TestResolveGoLicensesBinPathUsesGOBIN(t *testing.T) {
	binPath, err := resolveGoLicensesBinPath(func(key string) (string, error) {
		if key == "GOBIN" {
			return "/custom/gobin", nil
		}
		return "", errors.New("unexpected key: " + key)
	})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/custom/gobin", wantGoLicensesBinName()), binPath)
}

func TestResolveGoLicensesBinPathFallsBackToGopathBin(t *testing.T) {
	binPath, err := resolveGoLicensesBinPath(func(key string) (string, error) {
		switch key {
		case "GOBIN":
			return "", nil
		case "GOPATH":
			return "/home/user/go", nil
		default:
			return "", errors.New("unexpected key: " + key)
		}
	})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/user/go", "bin", wantGoLicensesBinName()), binPath)
}

func TestResolveGoLicensesBinPathPropagatesGOBINError(t *testing.T) {
	wantErr := errors.New("go env GOBIN failed")
	_, err := resolveGoLicensesBinPath(func(string) (string, error) { return "", wantErr })
	require.ErrorIs(t, err, wantErr)
}

func TestResolveGoLicensesBinPathPropagatesGOPATHError(t *testing.T) {
	wantErr := errors.New("go env GOPATH failed")
	_, err := resolveGoLicensesBinPath(func(key string) (string, error) {
		if key == "GOBIN" {
			return "", nil
		}
		return "", wantErr
	})
	require.ErrorIs(t, err, wantErr)
}

// TestEnsureGoLicensesFindsExistingBinary covers the fast path where
// go-licenses is already on PATH -- it must be returned as-is without
// attempting a `go install`. The stub is never executed (exec.LookPath only
// checks resolvability), so it's an empty file rather than a shell script,
// keeping this test Windows-safe: LookPath resolves ".exe" via PATHEXT on
// Windows and the executable bit on Unix.
func TestEnsureGoLicensesFindsExistingBinary(t *testing.T) {
	dir := t.TempDir()
	name := "go-licenses"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	stubPath := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(stubPath, nil, 0o755))

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	path, err := ensureGoLicenses("v1.6.0")
	require.NoError(t, err)
	assert.Equal(t, stubPath, path)
}

// TestEnsureGoLicensesRetriesOnTransientInstallFailure covers the retry loop
// around `go install`: the first two attempts fail (simulating a transient
// mid-stream network error, e.g. a sum.golang.org HTTP/2 reset during go.sum
// verification), the third succeeds, and ensureGoLicenses must return the
// resolved binary path without ever invoking the real 15s retry delay.
func TestEnsureGoLicensesRetriesOnTransientInstallFailure(t *testing.T) {
	gobin := t.TempDir()
	t.Setenv("GOBIN", gobin)
	// Deliberately does NOT already contain go-licenses, so ensureGoLicenses
	// falls through to the install path; go itself must stay resolvable for
	// resolveGoLicensesBinPath's real `go env GOBIN` call.
	t.Setenv("PATH", gobin+string(os.PathListSeparator)+os.Getenv("PATH"))

	name := "go-licenses"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binPath := filepath.Join(gobin, name)

	var installCalls []string
	attempt := 0
	origRunGoInstall := runGoInstall
	t.Cleanup(func() { runGoInstall = origRunGoInstall })
	runGoInstall = func(module string) error {
		attempt++
		installCalls = append(installCalls, module)
		if attempt < 3 {
			return errors.New("transient network error")
		}
		return os.WriteFile(binPath, nil, 0o755)
	}

	var sleeps []time.Duration
	origSleep := goInstallSleep
	t.Cleanup(func() { goInstallSleep = origSleep })
	goInstallSleep = func(d time.Duration) { sleeps = append(sleeps, d) }

	path, err := ensureGoLicenses("v1.6.0")

	require.NoError(t, err)
	assert.Equal(t, binPath, path)
	assert.Len(t, installCalls, 3, "must retry the failed attempts and stop once install succeeds")
	assert.Equal(t, []time.Duration{goInstallRetryDelay, goInstallRetryDelay}, sleeps, "sleeps only between attempts, never after the final one")
}

// TestEnsureGoLicensesFailsAfterExhaustingRetries covers the case where
// every attempt fails: ensureGoLicenses must give up after
// goInstallMaxAttempts and return a wrapped error instead of retrying
// forever.
func TestEnsureGoLicensesFailsAfterExhaustingRetries(t *testing.T) {
	gobin := t.TempDir()
	t.Setenv("GOBIN", gobin)
	t.Setenv("PATH", gobin+string(os.PathListSeparator)+os.Getenv("PATH"))

	attempt := 0
	wantErr := errors.New("persistent network error")
	origRunGoInstall := runGoInstall
	t.Cleanup(func() { runGoInstall = origRunGoInstall })
	runGoInstall = func(string) error {
		attempt++
		return wantErr
	}

	origSleep := goInstallSleep
	t.Cleanup(func() { goInstallSleep = origSleep })
	goInstallSleep = func(time.Duration) {}

	_, err := ensureGoLicenses("v1.6.0")

	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, goInstallMaxAttempts, attempt, "must stop after exactly goInstallMaxAttempts attempts")
}
