//go:build mage

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Build groups targets that produce the atmos binary.
type Build mg.Namespace

var (
	errUnsupportedBuildTarget = errors.New("mage: unsupported build target")
	errFIPSUnsupportedTarget  = errors.New("mage: GOFIPS140 does not support this GOOS/GOARCH")
)

const (
	// The Go package whose Version/Commit vars get stamped via -ldflags,
	// matching .goreleaser.yml.
	versionLdflagsPackage = "github.com/cloudposse/atmos/pkg/version"
	buildOutputDir        = "build"
	atmosBinaryName       = "atmos"
	directoryPermissions  = 0o755

	goosLinux   = "linux"
	goosWindows = "windows"
	goosDarwin  = "darwin"
	goarch386   = "386"
	goarchAMD64 = "amd64"

	// The 3-attempt/15s-backoff convention already used for artifact
	// downloads (.github/actions/download-artifact-retry). See
	// docs/fixes/2026-08-25-build-atmos-go-mod-download-retry.md.
	goModDownloadMaxAttempts = 3
	goModDownloadRetryDelay  = 15 * time.Second
)

// goModDownloadSleep is a seam so tests can exercise the retry loop without
// a real multi-second wait.
var goModDownloadSleep = time.Sleep

// Binary builds the atmos binary for target ("default", "linux", "windows",
// "macos", or "macos-intel"), embedding version (and the current commit, when
// building from a git checkout) via -ldflags. This is the Go implementation
// backing the `atmos build binary` custom command.
func (Build) Binary(target, version string) error {
	if target == "" {
		target = "default"
	}
	if version == "" {
		version = "test"
	}

	root, err := mageRepoRoot()
	if err != nil {
		return err
	}

	targetConfig, err := buildTargetConfig(target)
	if err != nil {
		return err
	}

	baseEnv := buildBaseEnv(root)

	if err := checkFIPSTargetSupported(targetConfig, baseEnv); err != nil {
		return err
	}

	// Best-effort: empty when not building from a git checkout, matching the
	// previous shell script's `git rev-parse HEAD 2>/dev/null || true`.
	commit, _ := sh.Output("git", "-C", root, "rev-parse", "HEAD")

	if err := runGoModDownload(root, baseEnv); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, buildOutputDir), directoryPermissions); err != nil {
		return fmt.Errorf("mage: mkdir %s: %w", buildOutputDir, err)
	}

	buildEnv := append([]string{}, baseEnv...)
	if targetConfig.GOOS != "" {
		buildEnv = append(buildEnv, "GOOS="+targetConfig.GOOS)
	}
	if targetConfig.GOARCH != "" {
		buildEnv = append(buildEnv, "GOARCH="+targetConfig.GOARCH)
	}

	ldflags := fmt.Sprintf(
		"-X '%s.Version=%s' -X '%s.Commit=%s'",
		versionLdflagsPackage, version, versionLdflagsPackage, strings.TrimSpace(commit),
	)

	return runIn(root, buildEnv, "go", "build", "-o", targetConfig.Output, "-v", "-ldflags", ldflags)
}

// buildTarget is the GOOS/GOARCH/output-path a build target resolves to.
// GOOS/GOARCH are empty when the target doesn't pin them, meaning the
// ambient GOOS/GOARCH environment (if any) takes effect instead.
type buildTarget struct {
	GOOS   string
	GOARCH string
	Output string
}

// buildTargetConfig maps a build target name to its buildTarget. The output
// path always uses the .exe suffix when the target resolves to GOOS=windows
// — whether windows was pinned explicitly (the "windows" target) or reached
// via ambient GOOS=windows on the "default" target — matching what `go
// build` itself names a Windows binary.
func buildTargetConfig(target string) (buildTarget, error) {
	ambientGOARCH := os.Getenv("GOARCH")
	switch target {
	case "default":
		goos := os.Getenv("GOOS")
		return buildTarget{GOOS: goos, GOARCH: ambientGOARCH, Output: outputPathFor(resolvedGOOS(goos))}, nil
	case "linux":
		return buildTarget{GOOS: goosLinux, GOARCH: ambientGOARCH, Output: outputPathFor(goosLinux)}, nil
	case "windows":
		return buildTarget{GOOS: goosWindows, GOARCH: ambientGOARCH, Output: outputPathFor(goosWindows)}, nil
	case "macos":
		return buildTarget{GOOS: goosDarwin, GOARCH: ambientGOARCH, Output: outputPathFor(goosDarwin)}, nil
	case "macos-intel":
		return buildTarget{GOOS: goosDarwin, GOARCH: goarchAMD64, Output: outputPathFor(goosDarwin)}, nil
	default:
		return buildTarget{}, fmt.Errorf("%w: %s (expected one of: default, linux, windows, macos, macos-intel)", errUnsupportedBuildTarget, target)
	}
}

// resolvedGOOS returns goos, falling back to the current process's GOOS
// (runtime.GOOS) when goos is empty — i.e. when a target doesn't pin GOOS
// and the ambient GOOS environment variable isn't set either, matching what
// `go build` itself would target.
func resolvedGOOS(goos string) string {
	if goos == "" {
		return runtime.GOOS
	}
	return goos
}

// resolvedGOARCH returns goarch, falling back to runtime.GOARCH when empty,
// mirroring resolvedGOOS.
func resolvedGOARCH(goarch string) string {
	if goarch == "" {
		return runtime.GOARCH
	}
	return goarch
}

// outputPathFor returns the build output path for goos: atmos.exe on
// Windows, atmos everywhere else.
func outputPathFor(goos string) string {
	name := atmosBinaryName
	if goos == goosWindows {
		name += ".exe"
	}
	return filepath.Join(buildOutputDir, name)
}

// buildBaseEnv returns the env overrides shared by `go mod download` and
// `go build` (but not GOOS/GOARCH, which only apply to the build step): CGO
// disabled, Go's native FIPS 140-3 crypto module linked in by default (see
// docs/prd/fips-140-mode.md), and reproducible builds from a worktree. Each
// only applies a default when the caller hasn't already set it, so an
// existing environment variable always wins.
func buildBaseEnv(root string) []string {
	env := map[string]string{}
	setDefault(env, "CGO_ENABLED", "0")
	setDefault(env, "GOFIPS140", "latest")
	if inWorktree(root) {
		setDefault(env, "GOFLAGS", "-buildvcs=false")
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// setDefault records key=value in env only when key isn't already set in the
// current process environment.
func setDefault(env map[string]string, key, value string) {
	if _, ok := os.LookupEnv(key); !ok {
		env[key] = value
	}
}

// checkFIPSTargetSupported rejects building for a GOOS/GOARCH combination
// that Go's native FIPS 140-3 module doesn't support at runtime, when the
// build will actually enable it. The windows/386 combination lacks a CPU
// jitter entropy source good enough for FIPS mode: crypto/internal/fips140's
// Supported() function panics on process init once GODEBUG=fips140=on (the
// default baked in by GOFIPS140=latest, see docs/prd/fips-140-mode.md), so a
// windows/386 binary built with FIPS enabled would crash immediately at
// startup rather than silently skip FIPS mode. The baseEnv slice is
// consulted first, since it carries the GOFIPS140 default this build is
// about to apply; the ambient environment is the fallback for a value the
// caller already set explicitly.
func checkFIPSTargetSupported(target buildTarget, baseEnv []string) error {
	goos := resolvedGOOS(target.GOOS)
	goarch := resolvedGOARCH(target.GOARCH)
	if goos != goosWindows || goarch != goarch386 {
		return nil
	}

	fips := envValue(baseEnv, "GOFIPS140")
	if fips == "" {
		fips = os.Getenv("GOFIPS140")
	}
	if fips == "" || fips == "off" {
		return nil
	}

	return fmt.Errorf("%w: GOOS=windows GOARCH=386 GOFIPS140=%s (set GOFIPS140=off to build this target anyway)",
		errFIPSUnsupportedTarget, fips)
}

// envValue returns the value of key in env (a "KEY=VALUE" slice as returned
// by buildBaseEnv), or "" if key isn't present.
func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, prefix); ok {
			return value
		}
	}
	return ""
}

// inWorktree reports whether root is a linked git worktree (as opposed to
// the repository's primary checkout), by comparing the per-worktree git-dir
// against the shared git-common-dir.
func inWorktree(root string) bool {
	gitDir, err := sh.Output("git", "-C", root, "rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	commonDir, err := sh.Output("git", "-C", root, "rev-parse", "--git-common-dir")
	if err != nil {
		return false
	}
	return strings.TrimSpace(gitDir) != strings.TrimSpace(commonDir)
}

// runGoModDownload runs `go mod download` in dir, retrying up to
// goModDownloadMaxAttempts times with a goModDownloadRetryDelay pause between
// attempts, since the Go module proxy occasionally resets a request
// mid-stream ("stream error: ... INTERNAL_ERROR received from peer") — a
// transient CDN/network blip, not a real dependency problem — which would
// otherwise fail the whole build outright on a single hiccup.
func runGoModDownload(dir string, env []string) error {
	var lastErr error
	for attempt := 1; attempt <= goModDownloadMaxAttempts; attempt++ {
		lastErr = runIn(dir, env, "go", "mod", "download")
		if lastErr == nil {
			return nil
		}
		if attempt == goModDownloadMaxAttempts {
			break
		}
		fmt.Fprintf(os.Stderr, "go mod download failed (attempt %d/%d), retrying in %s...\n",
			attempt, goModDownloadMaxAttempts, goModDownloadRetryDelay)
		goModDownloadSleep(goModDownloadRetryDelay)
	}
	return fmt.Errorf("mage: go mod download failed after %d attempts: %w", goModDownloadMaxAttempts, lastErr)
}

// runIn runs name with args in dir, with extraEnv appended on top of the
// current process environment (so later duplicate keys win), streaming
// output directly to this process's stdout/stderr.
func runIn(dir string, extraEnv []string, name string, args ...string) error {
	cmd := exec.Command(name, args...) // #nosec G204 -- fixed command names, mage-target-controlled args.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mage: run %s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
