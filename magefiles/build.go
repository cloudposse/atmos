//go:build mage

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Build groups targets that produce the atmos binary.
type Build mg.Namespace

var errUnsupportedBuildTarget = errors.New("mage: unsupported build target")

const (
	// The Go package whose Version/Commit vars get stamped via -ldflags,
	// matching .goreleaser.yml.
	versionLdflagsPackage = "github.com/cloudposse/atmos/pkg/version"
	buildOutputDir        = "build"
	atmosBinaryName       = "atmos"
	directoryPermissions  = 0o755
)

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

	// Best-effort: empty when not building from a git checkout, matching the
	// previous shell script's `git rev-parse HEAD 2>/dev/null || true`.
	commit, _ := sh.Output("git", "-C", root, "rev-parse", "HEAD")

	baseEnv := buildBaseEnv(root)

	if err := runIn(root, baseEnv, "go", "mod", "download"); err != nil {
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

// buildTargetConfig maps a build target name to its buildTarget.
func buildTargetConfig(target string) (buildTarget, error) {
	ambientGOARCH := os.Getenv("GOARCH")
	defaultOutput := filepath.Join(buildOutputDir, atmosBinaryName)
	switch target {
	case "default":
		return buildTarget{GOOS: os.Getenv("GOOS"), GOARCH: ambientGOARCH, Output: defaultOutput}, nil
	case "linux":
		return buildTarget{GOOS: "linux", GOARCH: ambientGOARCH, Output: defaultOutput}, nil
	case "windows":
		return buildTarget{GOOS: "windows", GOARCH: ambientGOARCH, Output: filepath.Join(buildOutputDir, atmosBinaryName+".exe")}, nil
	case "macos":
		return buildTarget{GOOS: "darwin", GOARCH: ambientGOARCH, Output: defaultOutput}, nil
	case "macos-intel":
		return buildTarget{GOOS: "darwin", GOARCH: "amd64", Output: defaultOutput}, nil
	default:
		return buildTarget{}, fmt.Errorf("%w: %s (expected one of: default, linux, windows, macos, macos-intel)", errUnsupportedBuildTarget, target)
	}
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
