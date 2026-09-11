package tests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/otiai10/copy"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/toolchain/installer"
	"github.com/cloudposse/atmos/tests/testhelpers/gitconfigenv"
)

// canaryTimeout bounds each live-GitHub canary so a hung connection fails fast instead of eating
// the whole test budget; these are small, targeted operations and should complete in seconds.
const canaryTimeout = 2 * time.Minute

// contextTFMarker is a stable, unlikely-to-change substring of
// github.com/cloudposse/terraform-null-label's exports/context.tf (the header comment every
// consumer of that file is warned not to edit), used to assert real content actually landed
// rather than an empty or truncated file.
const contextTFMarker = "ONLY EDIT THIS FILE IN github.com/cloudposse/terraform-null-label"

// transientLiveGitHubPatterns are stderr signatures of a network/service condition outside a
// live-GitHub canary's control: DNS/connect failures, timeouts, TLS trust failures, and
// rate-limit/5xx responses. This mirrors the "transient -> skip, real semantics -> fail" pattern
// pkg/github/releases_test.go's isGitHubTransientError already uses for in-process live-network
// tests (see docs/fixes/2026-08-10-github-transient-error-tls-cert-flake.md); these canaries
// drive a subprocess instead of a typed Go error, so classification works off captured stderr
// text.
var transientLiveGitHubPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)could not resolve host`),
	regexp.MustCompile(`(?i)could not connect`),
	regexp.MustCompile(`(?i)failed to connect`),
	regexp.MustCompile(`(?i)connection refused`),
	regexp.MustCompile(`(?i)connection reset`),
	regexp.MustCompile(`(?i)\btimeout\b`),
	regexp.MustCompile(`(?i)timed out`),
	regexp.MustCompile(`(?i)\btls\b`),
	regexp.MustCompile(`(?i)x509`),
	regexp.MustCompile(`\b429\b`),
	regexp.MustCompile(`(?i)rate limit`),
	regexp.MustCompile(`\b50[0-9]\b`),
}

// classifyLiveGitHubFailure reports whether stderr describes a transient condition outside a
// live-GitHub canary's control, as opposed to a real failure (a genuine 401/403/404, or a bug in
// atmos) worth failing the build over. Callers should t.Skip on transient and t.Fatal otherwise.
func classifyLiveGitHubFailure(stderr string) (transient bool, matchedPattern string) {
	for _, pattern := range transientLiveGitHubPatterns {
		if pattern.MatchString(stderr) {
			return true, pattern.String()
		}
	}
	return false, ""
}

// skipOrFailLiveGitHubCanary classifies a canary's subprocess failure: skip on a transient
// network/service condition, fail the test otherwise. Call sites pass the *exec.Cmd error and its
// captured stderr.
func skipOrFailLiveGitHubCanary(t *testing.T, action string, err error, stderr string) {
	t.Helper()

	if err == nil {
		return
	}
	if transient, pattern := classifyLiveGitHubFailure(stderr); transient {
		t.Skipf("skipping %s: transient condition reaching live GitHub (matched %q): %v\nstderr:\n%s", action, pattern, err, stderr)
	}
	t.Fatalf("%s failed: %v\nstderr:\n%s", action, err, stderr)
}

// copyCanaryFixture copies tests/fixtures/scenarios/<name> into a fresh t.TempDir() so concurrent
// or repeated canary runs never share (and clobber) vendored output.
func copyCanaryFixture(t *testing.T, name string) string {
	t.Helper()

	src := filepath.Join(".", "fixtures", "scenarios", name)
	dst := filepath.Join(t.TempDir(), name)
	require.NoError(t, copy.Copy(src, dst), "copy fixture %s", name)
	return dst
}

// githubCanaryEnv returns a copy of base (typically an *exec.Cmd's already-prepared Env from
// AtmosRunner.CommandContext, which carries PATH/GOCOVERDIR setup worth preserving) suitable for
// driving atmos against real, live github.com:
//
//   - GIT_CONFIG_GLOBAL is pointed at an empty file, so the local git mirror's insteadOf rules
//     TestMain exported through it (tests/testhelpers/gitmirror.WriteGitConfig) do not apply:
//     even a canary targeting cloudposse/atmos itself must hit the real network here.
//   - When authenticated is false, every GitHub token env var is blanked and GH_CONFIG_DIR points
//     at an empty temp dir, defeating the `gh auth token` CLI fallback
//     (pkg/downloader/custom_git_detector.go resolveToken, pkg/github.GetGitHubTokenFromCLI) so
//     the canary genuinely exercises the unauthenticated path.
//   - When authenticated is true and GITHUB_TOKEN is set, an http.https://github.com/.extraheader
//     basic-auth entry is added, mirroring runCLICommandTest's own token injection.
func githubCanaryEnv(t *testing.T, base []string, authenticated bool) []string {
	t.Helper()

	existingEntries := gitconfigenv.ReadEntries(base)
	env := removeEnvPrefixed(base, "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_")

	// TestMain delivers the local git mirror's url.*.insteadOf rules through GIT_CONFIG_GLOBAL
	// (tests/testhelpers/gitmirror.WriteGitConfig). A canary must reach the real github.com, so
	// point git at an empty global config instead; atmos never reads that variable.
	env = removeEnvKeys(env, "GIT_CONFIG_GLOBAL")
	env = append(env, "GIT_CONFIG_GLOBAL="+emptyGitConfigFile(t))

	gitEntries := []gitconfigenv.GitConfigEntry{
		// Disable credential helper (prevents osxkeychain hangs/popups), mirroring
		// runCLICommandTest.
		{Key: "credential.helper", Value: ""},
	}

	if authenticated {
		if token := os.Getenv("GITHUB_TOKEN"); token != "" {
			credential := "x-access-token:" + token
			basicAuth := base64.StdEncoding.EncodeToString([]byte(credential))
			iolib.RegisterSecret(credential)
			gitEntries = append(gitEntries, gitconfigenv.GitConfigEntry{
				Key:   "http.https://github.com/.extraheader",
				Value: "AUTHORIZATION: basic " + basicAuth,
			})
		}
	} else {
		env = removeEnvKeys(env, "GITHUB_TOKEN", "ATMOS_GITHUB_TOKEN", "ATMOS_PRO_GITHUB_TOKEN", "GH_TOKEN", "GH_CONFIG_DIR")
		env = append(
			env,
			"GITHUB_TOKEN=",
			"ATMOS_GITHUB_TOKEN=",
			"ATMOS_PRO_GITHUB_TOKEN=",
			"GH_TOKEN=",
			"GH_CONFIG_DIR="+t.TempDir(),
		)
	}

	gitConfigVars := map[string]string{}
	gitconfigenv.AppendEntries(gitConfigVars, existingEntries, gitEntries...)
	for k, v := range gitConfigVars {
		env = append(env, k+"="+v)
	}

	return env
}

// removeEnvKeys returns a copy of env with every "KEY=..." entry removed whose key exactly
// matches one of keys.
func removeEnvKeys(env []string, keys ...string) []string {
	kept := make([]string, 0, len(env))
outer:
	for _, kv := range env {
		for _, key := range keys {
			if kv == key || strings.HasPrefix(kv, key+"=") {
				continue outer
			}
		}
		kept = append(kept, kv)
	}
	return kept
}

// removeEnvPrefixed returns a copy of env with every entry removed whose "KEY=" text starts with
// one of prefixes.
func removeEnvPrefixed(env []string, prefixes ...string) []string {
	kept := make([]string, 0, len(env))
outer:
	for _, kv := range env {
		for _, prefix := range prefixes {
			if strings.HasPrefix(kv, prefix) {
				continue outer
			}
		}
		kept = append(kept, kv)
	}
	return kept
}

// setEnvVar returns a copy of env with key set to value, replacing any existing entry for key.
func setEnvVar(env []string, key, value string) []string {
	return append(removeEnvKeys(env, key), key+"="+value)
}

// findFileNamed walks root and returns the path of the first regular file named exactly name, or
// "" if none is found. Used to locate an installed toolchain binary without hardcoding the
// installer's internal directory layout.
func findFileNamed(root, name string) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil //nolint:nilerr // Best-effort search; a walk error just means "not found here".
		}
		if !d.IsDir() && d.Name() == name {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// TestLiveGitHubCanary_UnauthenticatedVendorPull vendors a single tiny file from a real, external
// cloudposse repo (never the local git mirror, which only contains cloudposse/atmos) over a
// genuinely unauthenticated HTTPS clone, keeping the acceptance suite honest about the live,
// unauthenticated GitHub path that tests/testhelpers/gitmirror and the PR3 HTTP mock otherwise
// remove from the default run.
func TestLiveGitHubCanary_UnauthenticatedVendorPull(t *testing.T) {
	SkipIfShort(t)
	RequireLiveGitHub(t)
	ensureAtmosRunner(t)

	fixtureDir := copyCanaryFixture(t, "live-github-canary-vendor")
	t.Chdir(fixtureDir)

	ctx, cancel := context.WithTimeout(context.Background(), canaryTimeout)
	defer cancel()

	cmd := atmosRunner.CommandContext(ctx, "vendor", "pull")
	cmd.Env = githubCanaryEnv(t, cmd.Env, false)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	skipOrFailLiveGitHubCanary(t, "unauthenticated vendor pull", err, stderr.String())

	vendored := filepath.Join(fixtureDir, "components", "terraform", "live-github-canary", "exports", "context.tf")
	content, readErr := os.ReadFile(vendored)
	require.NoError(t, readErr, "expected vendored file at %s", vendored)
	require.Contains(t, string(content), contextTFMarker)
}

// TestLiveGitHubCanary_AuthenticatedVendorPull mirrors
// TestLiveGitHubCanary_UnauthenticatedVendorPull but keeps GITHUB_TOKEN, exercising the
// authenticated live-GitHub clone path (token injected via the extraheader git config entry).
// Skips when GITHUB_TOKEN is not set.
func TestLiveGitHubCanary_AuthenticatedVendorPull(t *testing.T) {
	SkipIfShort(t)
	RequireLiveGitHubAuthenticated(t)
	ensureAtmosRunner(t)

	fixtureDir := copyCanaryFixture(t, "live-github-canary-vendor")
	t.Chdir(fixtureDir)

	ctx, cancel := context.WithTimeout(context.Background(), canaryTimeout)
	defer cancel()

	cmd := atmosRunner.CommandContext(ctx, "vendor", "pull")
	cmd.Env = githubCanaryEnv(t, cmd.Env, true)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	skipOrFailLiveGitHubCanary(t, "authenticated vendor pull", err, stderr.String())

	vendored := filepath.Join(fixtureDir, "components", "terraform", "live-github-canary", "exports", "context.tf")
	content, readErr := os.ReadFile(vendored)
	require.NoError(t, readErr, "expected vendored file at %s", vendored)
	require.Contains(t, string(content), contextTFMarker)
}

// TestLiveGitHubCanary_ToolchainInstall installs a single tiny tool (peteretelej/tree, already
// pinned in .tool-versions) from a real GitHub release asset into an isolated cache directory,
// unauthenticated. This is the exact code path that produced the HTTP 404 CI bootstrap failure
// this PR's .github/actions/ci-toolchain retry addresses.
func TestLiveGitHubCanary_ToolchainInstall(t *testing.T) {
	SkipIfShort(t)
	RequireLiveGitHub(t)
	ensureAtmosRunner(t)

	cacheDir := t.TempDir()
	t.Chdir(t.TempDir())

	ctx, cancel := context.WithTimeout(context.Background(), canaryTimeout)
	defer cancel()

	cmd := atmosRunner.CommandContext(ctx, "toolchain", "install", "peteretelej/tree@v1.3.0")
	cmd.Env = githubCanaryEnv(t, cmd.Env, false)
	cmd.Env = setEnvVar(cmd.Env, "ATMOS_XDG_CACHE_HOME", cacheDir)
	cmd.Env = setEnvVar(cmd.Env, "XDG_CACHE_HOME", cacheDir)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	skipOrFailLiveGitHubCanary(t, "unauthenticated toolchain install", err, stderr.String())

	binaryName := installer.EnsureWindowsExeExtension("tree")
	binaryPath := findFileNamed(cacheDir, binaryName)
	require.NotEmpty(t, binaryPath, "expected %s to be installed somewhere under %s", binaryName, cacheDir)

	info, statErr := os.Stat(binaryPath)
	require.NoError(t, statErr)
	require.False(t, info.IsDir())
}

// TestLiveGitHubCanary_UnauthenticatedRawInclude resolves a `!include.raw` YAML function against
// a real raw.githubusercontent.com URL, unauthenticated -- the plain HTTP fetch path
// tests/yaml_functions_include_test.go otherwise always mocks via httpmock.GitHubMockServer.
func TestLiveGitHubCanary_UnauthenticatedRawInclude(t *testing.T) {
	SkipIfShort(t)
	RequireLiveGitHub(t)
	ensureAtmosRunner(t)

	fixtureDir := copyCanaryFixture(t, "live-github-canary-include")
	t.Chdir(fixtureDir)

	ctx, cancel := context.WithTimeout(context.Background(), canaryTimeout)
	defer cancel()

	cmd := atmosRunner.CommandContext(ctx, "describe", "component", "component-1", "--stack", "nonprod", "--format", "json")
	cmd.Env = githubCanaryEnv(t, cmd.Env, false)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	skipOrFailLiveGitHubCanary(t, "unauthenticated raw include", err, stderr.String())

	var described map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &described), "describe component output: %s", stdout.String())

	vars, ok := described["vars"].(map[string]any)
	require.True(t, ok, "vars should be a map, got %T", described["vars"])

	rawContext, ok := vars["raw_context"].(string)
	require.True(t, ok, "vars.raw_context should be a string, got %T", vars["raw_context"])
	require.Contains(t, rawContext, contextTFMarker)
}
