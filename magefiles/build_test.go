//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envSliceToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			out[name] = value
		}
	}
	return out
}

// unsetEnv removes key from the process environment for the duration of the
// test, restoring its prior value (or absence) on cleanup. Setting a
// variable via t.Setenv can't express "unset": an empty string still leaves
// the key present for os.LookupEnv, and still leaks an empty-valued entry
// into subprocess environments built from os.Environ(). This repo's own dev
// shell ambiently exports GOFLAGS=-buildvcs=false for worktrees, so tests
// asserting a variable's absence-by-default need genuine unset semantics,
// not just an empty value, to stay correct regardless of the ambient shell.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	original, wasSet := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestBuildTargetConfig(t *testing.T) {
	t.Run("unsupported target", func(t *testing.T) {
		_, err := buildTargetConfig("bogus")
		require.ErrorIs(t, err, errUnsupportedBuildTarget)
		assert.Contains(t, err.Error(), "bogus")
	})

	cases := []struct {
		name        string
		target      string
		ambientGOOS string
		ambientArch string
		wantGOOS    string
		wantGOARCH  string
		wantOutput  string
	}{
		{
			name:       "default with no ambient GOOS/GOARCH",
			target:     "default",
			wantOutput: filepath.Join("build", "atmos"),
		},
		{
			name:        "default passes through ambient GOOS",
			target:      "default",
			ambientGOOS: "linux",
			wantGOOS:    "linux",
			wantOutput:  filepath.Join("build", "atmos"),
		},
		{
			// Regression test: the default target used to hardcode
			// build/atmos regardless of GOOS, so an ambient GOOS=windows
			// build (e.g. cross-compiling without the explicit "windows"
			// target) produced a Windows PE binary at a Unix-style output
			// path instead of build/atmos.exe.
			name:        "default resolves .exe output when ambient GOOS is windows",
			target:      "default",
			ambientGOOS: "windows",
			wantGOOS:    "windows",
			wantOutput:  filepath.Join("build", "atmos.exe"),
		},
		{
			name:       "linux pins GOOS",
			target:     "linux",
			wantGOOS:   "linux",
			wantOutput: filepath.Join("build", "atmos"),
		},
		{
			name:       "windows pins GOOS and .exe output",
			target:     "windows",
			wantGOOS:   "windows",
			wantOutput: filepath.Join("build", "atmos.exe"),
		},
		{
			name:       "macos pins GOOS",
			target:     "macos",
			wantGOOS:   "darwin",
			wantOutput: filepath.Join("build", "atmos"),
		},
		{
			name:        "macos-intel pins GOARCH regardless of ambient value",
			target:      "macos-intel",
			ambientArch: "arm64",
			wantGOOS:    "darwin",
			wantGOARCH:  "amd64",
			wantOutput:  filepath.Join("build", "atmos"),
		},
		{
			name:        "linux passes through ambient GOARCH",
			target:      "linux",
			ambientArch: "arm64",
			wantGOOS:    "linux",
			wantGOARCH:  "arm64",
			wantOutput:  filepath.Join("build", "atmos"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOS", tc.ambientGOOS)
			t.Setenv("GOARCH", tc.ambientArch)

			config, err := buildTargetConfig(tc.target)
			require.NoError(t, err)
			assert.Equal(t, tc.wantGOOS, config.GOOS)
			assert.Equal(t, tc.wantGOARCH, config.GOARCH)
			assert.Equal(t, tc.wantOutput, config.Output)
		})
	}
}

func TestCheckFIPSTargetSupported(t *testing.T) {
	t.Run("rejects windows/386 when GOFIPS140 will be enabled", func(t *testing.T) {
		err := checkFIPSTargetSupported(buildTarget{GOOS: "windows", GOARCH: "386"}, []string{"GOFIPS140=latest"})
		require.ErrorIs(t, err, errFIPSUnsupportedTarget)
		assert.Contains(t, err.Error(), "GOOS=windows")
		assert.Contains(t, err.Error(), "GOARCH=386")
	})

	t.Run("allows windows/386 when GOFIPS140 is explicitly off", func(t *testing.T) {
		err := checkFIPSTargetSupported(buildTarget{GOOS: "windows", GOARCH: "386"}, []string{"GOFIPS140=off"})
		require.NoError(t, err)
	})

	t.Run("allows windows/386 when GOFIPS140 is off in the ambient environment", func(t *testing.T) {
		t.Setenv("GOFIPS140", "off")
		err := checkFIPSTargetSupported(buildTarget{GOOS: "windows", GOARCH: "386"}, nil)
		require.NoError(t, err)
	})

	t.Run("allows windows/amd64 under FIPS", func(t *testing.T) {
		err := checkFIPSTargetSupported(buildTarget{GOOS: "windows", GOARCH: "amd64"}, []string{"GOFIPS140=latest"})
		require.NoError(t, err)
	})

	t.Run("allows any target when GOOS/GOARCH aren't pinned and the host isn't windows/386", func(t *testing.T) {
		err := checkFIPSTargetSupported(buildTarget{}, []string{"GOFIPS140=latest"})
		require.NoError(t, err)
	})

	// Table-driven coverage for every GOOS/GOARCH combination
	// crypto/internal/fips140.Supported() rejects at runtime init (see
	// checkFIPSTargetSupported's doc comment for the source citation),
	// beyond the windows/386 case already covered above.
	rejectedCases := []struct {
		name   string
		target buildTarget
	}{
		{name: "js/wasm", target: buildTarget{GOOS: "js", GOARCH: "wasm"}},
		{name: "wasip1/wasm", target: buildTarget{GOOS: "wasip1", GOARCH: "wasm"}},
		{name: "openbsd/amd64", target: buildTarget{GOOS: "openbsd", GOARCH: "amd64"}},
		{name: "openbsd/arm64", target: buildTarget{GOOS: "openbsd", GOARCH: "arm64"}},
		{name: "aix/ppc64", target: buildTarget{GOOS: "aix", GOARCH: "ppc64"}},
	}
	for _, tc := range rejectedCases {
		t.Run("rejects "+tc.name+" when GOFIPS140 will be enabled", func(t *testing.T) {
			err := checkFIPSTargetSupported(tc.target, []string{"GOFIPS140=latest"})
			require.ErrorIs(t, err, errFIPSUnsupportedTarget)
			assert.Contains(t, err.Error(), "GOOS="+tc.target.GOOS)
			assert.Contains(t, err.Error(), "GOARCH="+tc.target.GOARCH)
		})

		t.Run("allows "+tc.name+" when GOFIPS140 is explicitly off", func(t *testing.T) {
			err := checkFIPSTargetSupported(tc.target, []string{"GOFIPS140=off"})
			require.NoError(t, err)
		})
	}

	supportedCases := []struct {
		name   string
		target buildTarget
	}{
		{name: "linux/amd64", target: buildTarget{GOOS: "linux", GOARCH: "amd64"}},
		{name: "darwin/arm64", target: buildTarget{GOOS: "darwin", GOARCH: "arm64"}},
		{name: "freebsd/amd64", target: buildTarget{GOOS: "freebsd", GOARCH: "amd64"}},
	}
	for _, tc := range supportedCases {
		t.Run("allows "+tc.name+" under FIPS", func(t *testing.T) {
			err := checkFIPSTargetSupported(tc.target, []string{"GOFIPS140=latest"})
			require.NoError(t, err)
		})
	}
}

// TestFipsUnsupportedTarget covers fipsUnsupportedTarget's decision logic
// directly, independent of the GOFIPS140-enabled gating in
// checkFIPSTargetSupported.
func TestFipsUnsupportedTarget(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		goarch string
		want   bool
	}{
		{name: "windows/386 rejected", goos: "windows", goarch: "386", want: true},
		{name: "windows/amd64 allowed", goos: "windows", goarch: "amd64", want: false},
		{name: "windows/arm allowed (dropped by Go itself, not FIPS)", goos: "windows", goarch: "arm", want: false},
		{name: "js/wasm rejected", goos: "js", goarch: "wasm", want: true},
		{name: "wasip1/wasm rejected", goos: "wasip1", goarch: "wasm", want: true},
		{name: "openbsd/amd64 rejected", goos: "openbsd", goarch: "amd64", want: true},
		{name: "openbsd/arm64 rejected", goos: "openbsd", goarch: "arm64", want: true},
		{name: "aix/ppc64 rejected", goos: "aix", goarch: "ppc64", want: true},
		{name: "linux/amd64 allowed", goos: "linux", goarch: "amd64", want: false},
		{name: "darwin/arm64 allowed", goos: "darwin", goarch: "arm64", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fipsUnsupportedTarget(tt.goos, tt.goarch))
		})
	}
}

func TestSetDefault(t *testing.T) {
	t.Run("sets a default when the key is unset", func(t *testing.T) {
		key := "ATMOS_MAGEFILES_TEST_UNSET_VAR"
		_, alreadySet := os.LookupEnv(key)
		require.False(t, alreadySet, "test precondition: %s must not be set", key)

		env := map[string]string{}
		setDefault(env, key, "fallback")
		assert.Equal(t, "fallback", env[key])
	})

	t.Run("leaves an existing value untouched", func(t *testing.T) {
		key := "ATMOS_MAGEFILES_TEST_SET_VAR"
		t.Setenv(key, "explicit")

		env := map[string]string{}
		setDefault(env, key, "fallback")
		_, ok := env[key]
		assert.False(t, ok, "setDefault must not override an already-set environment variable")
	})
}

func TestInWorktree(t *testing.T) {
	t.Run("not a git repository", func(t *testing.T) {
		assert.False(t, inWorktree(t.TempDir()))
	})

	t.Run("primary checkout is not a worktree", func(t *testing.T) {
		root := initGitRepoFixture(t)
		assert.False(t, inWorktree(root))
	})

	t.Run("linked worktree", func(t *testing.T) {
		root := initGitRepoFixture(t)
		worktreeDir := filepath.Join(t.TempDir(), "wt")
		runGit(t, root, "worktree", "add", "-b", "wt-branch-inworktree", worktreeDir)
		assert.True(t, inWorktree(worktreeDir))
	})
}

func TestBuildBaseEnv(t *testing.T) {
	t.Run("defaults outside a worktree", func(t *testing.T) {
		unsetEnv(t, "GOFLAGS")
		root := initGitRepoFixture(t)
		env := envSliceToMap(buildBaseEnv(root))
		assert.Equal(t, "0", env["CGO_ENABLED"])
		assert.Equal(t, "latest", env["GOFIPS140"])
		_, hasGoflags := env["GOFLAGS"]
		assert.False(t, hasGoflags, "GOFLAGS should only default inside a worktree")
	})

	t.Run("respects existing overrides", func(t *testing.T) {
		root := initGitRepoFixture(t)
		t.Setenv("CGO_ENABLED", "1")
		t.Setenv("GOFIPS140", "off")

		env := envSliceToMap(buildBaseEnv(root))
		_, hasCgo := env["CGO_ENABLED"]
		assert.False(t, hasCgo, "an already-set CGO_ENABLED must not be re-added to the override map")
		_, hasFips := env["GOFIPS140"]
		assert.False(t, hasFips, "an already-set GOFIPS140 must not be re-added to the override map")
	})

	t.Run("defaults GOFLAGS inside a worktree", func(t *testing.T) {
		unsetEnv(t, "GOFLAGS")
		root := initGitRepoFixture(t)
		worktreeDir := filepath.Join(t.TempDir(), "wt")
		runGit(t, root, "worktree", "add", "-b", "wt-branch-baseenv", worktreeDir)

		env := envSliceToMap(buildBaseEnv(worktreeDir))
		assert.Equal(t, "-buildvcs=false", env["GOFLAGS"])
	})
}

func TestRunIn(t *testing.T) {
	t.Run("success streams args, env, and cwd through", func(t *testing.T) {
		argsFile := setUpFakePathBinary(t, "footool")
		dir := t.TempDir()

		require.NoError(t, runIn(dir, []string{"FOO=bar"}, "footool", "arg1", "arg2"))

		assert.Equal(t, []string{"arg1", "arg2"}, readFakeBinArgs(t, argsFile))
		assert.Equal(t, "bar", readFakeBinEnv(t, argsFile)["FOO"])
		wantDir, err := filepath.EvalSymlinks(dir)
		require.NoError(t, err)
		assert.Equal(t, wantDir, readFakeBinCwd(t, argsFile))
	})

	t.Run("propagates subprocess failure", func(t *testing.T) {
		setUpFakePathBinary(t, "footool")
		t.Setenv(fakeBinExitEnv, "1")

		err := runIn(t.TempDir(), nil, "footool", "boom")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "footool boom")
	})
}

func TestBuildBinary(t *testing.T) {
	t.Run("propagates repo-root resolution failure", func(t *testing.T) {
		t.Chdir(t.TempDir())
		err := Build{}.Binary("default", "test")
		require.ErrorIs(t, err, errMageRepoRootNotFound)
	})

	t.Run("propagates unsupported target error", func(t *testing.T) {
		root := initGitRepoFixture(t)
		t.Chdir(root)
		err := Build{}.Binary("bogus", "test")
		require.ErrorIs(t, err, errUnsupportedBuildTarget)
	})

	t.Run("propagates go mod download failure without attempting go build", func(t *testing.T) {
		withNoSleep(t)
		root := initGitRepoFixture(t)
		argsFile := setUpFakePathBinaryFailingNTimes(t, goModDownloadMaxAttempts+5)
		t.Chdir(root)

		err := Build{}.Binary("default", "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mod download")
		assert.Equal(t, []string{"mod", "download"}, readFakeBinArgs(t, argsFile),
			"go build must not run once go mod download has exhausted its retries")
		assert.Equal(t, goModDownloadMaxAttempts, readFakeBinInvocationCount(t, argsFile))
	})

	t.Run("builds with default CGO/FIPS env, target GOOS, and version/commit ldflags", func(t *testing.T) {
		unsetEnv(t, "GOARCH")
		root := initGitRepoFixture(t)
		wantCommit, err := execGitOutput(root, "rev-parse", "HEAD")
		require.NoError(t, err)
		argsFile := setUpFakePathBinary(t, "go")
		t.Chdir(root)

		require.NoError(t, Build{}.Binary("linux", "v1.2.3"))

		wantLdflags := fmt.Sprintf("-X '%s.Version=v1.2.3' -X '%s.Commit=%s'",
			versionLdflagsPackage, versionLdflagsPackage, wantCommit)
		assert.Equal(t, []string{"build", "-o", filepath.Join("build", "atmos"), "-v", "-ldflags", wantLdflags},
			readFakeBinArgs(t, argsFile))

		env := readFakeBinEnv(t, argsFile)
		assert.Equal(t, "0", env["CGO_ENABLED"])
		assert.Equal(t, "latest", env["GOFIPS140"])
		assert.Equal(t, "linux", env["GOOS"])
		_, hasArch := env["GOARCH"]
		assert.False(t, hasArch, "GOARCH must not be forced when the ambient environment doesn't set it")

		// Resolve both sides (e.g. macOS's /var -> /private/var) before
		// comparing: which side the OS resolves symlinks on isn't something
		// worth pinning down here, only that they name the same directory.
		wantRoot, err := filepath.EvalSymlinks(root)
		require.NoError(t, err)
		gotCwd, err := filepath.EvalSymlinks(readFakeBinCwd(t, argsFile))
		require.NoError(t, err)
		assert.Equal(t, wantRoot, gotCwd)

		info, err := os.Stat(filepath.Join(root, "build"))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("windows target builds an .exe and defaults target/version when empty", func(t *testing.T) {
		root := initGitRepoFixture(t)
		argsFile := setUpFakePathBinary(t, "go")
		t.Chdir(root)

		require.NoError(t, Build{}.Binary("windows", ""))

		args := readFakeBinArgs(t, argsFile)
		require.Len(t, args, 6)
		assert.Equal(t, filepath.Join("build", "atmos.exe"), args[2])
		assert.Contains(t, args[5], "Version=test")
		assert.Equal(t, "windows", readFakeBinEnv(t, argsFile)["GOOS"])
	})

	t.Run("macos-intel pins GOARCH=amd64 regardless of the ambient value", func(t *testing.T) {
		root := initGitRepoFixture(t)
		argsFile := setUpFakePathBinary(t, "go")
		t.Setenv("GOARCH", "arm64")
		t.Chdir(root)

		require.NoError(t, Build{}.Binary("macos-intel", "test"))

		env := readFakeBinEnv(t, argsFile)
		assert.Equal(t, "darwin", env["GOOS"])
		assert.Equal(t, "amd64", env["GOARCH"])
	})

	t.Run("respects an existing CGO_ENABLED/GOFIPS140 override", func(t *testing.T) {
		root := initGitRepoFixture(t)
		argsFile := setUpFakePathBinary(t, "go")
		t.Setenv("CGO_ENABLED", "1")
		t.Setenv("GOFIPS140", "off")
		t.Chdir(root)

		require.NoError(t, Build{}.Binary("default", "test"))

		env := readFakeBinEnv(t, argsFile)
		assert.Equal(t, "1", env["CGO_ENABLED"])
		assert.Equal(t, "off", env["GOFIPS140"])
	})

	t.Run("rejects GOOS=windows GOARCH=386 under the default FIPS-enabled build without invoking go", func(t *testing.T) {
		root := initGitRepoFixture(t)
		argsFile := setUpFakePathBinary(t, "go")
		t.Setenv("GOOS", "windows")
		t.Setenv("GOARCH", "386")
		t.Chdir(root)

		err := Build{}.Binary("default", "test")
		require.ErrorIs(t, err, errFIPSUnsupportedTarget)
		_, statErr := os.Stat(argsFile)
		assert.True(t, os.IsNotExist(statErr), "go must never be invoked once the FIPS/target check rejects the build")
	})

	t.Run("builds GOOS=windows GOARCH=386 when GOFIPS140=off is set explicitly", func(t *testing.T) {
		root := initGitRepoFixture(t)
		argsFile := setUpFakePathBinary(t, "go")
		t.Setenv("GOOS", "windows")
		t.Setenv("GOARCH", "386")
		t.Setenv("GOFIPS140", "off")
		t.Chdir(root)

		require.NoError(t, Build{}.Binary("default", "test"))

		args := readFakeBinArgs(t, argsFile)
		assert.Equal(t, filepath.Join("build", "atmos.exe"), args[2])
		assert.Equal(t, "off", readFakeBinEnv(t, argsFile)["GOFIPS140"])
	})

	t.Run("sets GOFLAGS=-buildvcs=false when building from a worktree", func(t *testing.T) {
		unsetEnv(t, "GOFLAGS")
		root := initGitRepoFixture(t)
		worktreeDir := filepath.Join(t.TempDir(), "wt")
		runGit(t, root, "worktree", "add", "-b", "wt-branch-buildbinary", worktreeDir)
		require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "go.mod"), []byte(rootModuleDecl+"\n\ngo 1.26\n"), 0o644))
		argsFile := setUpFakePathBinary(t, "go")
		t.Chdir(worktreeDir)

		require.NoError(t, Build{}.Binary("default", "test"))

		assert.Equal(t, "-buildvcs=false", readFakeBinEnv(t, argsFile)["GOFLAGS"])
	})
}

// withNoSleep overrides goModDownloadSleep to a no-op for the duration of
// the test, so retry-loop tests don't actually wait goModDownloadRetryDelay
// between attempts.
func withNoSleep(t *testing.T) {
	t.Helper()
	original := goModDownloadSleep
	goModDownloadSleep = func(time.Duration) {}
	t.Cleanup(func() { goModDownloadSleep = original })
}

func TestRunGoModDownload(t *testing.T) {
	t.Run("succeeds on the first attempt", func(t *testing.T) {
		withNoSleep(t)
		argsFile := setUpFakePathBinaryFailingNTimes(t, 0)

		require.NoError(t, runGoModDownload(t.TempDir(), nil))

		assert.Equal(t, 1, readFakeBinInvocationCount(t, argsFile))
	})

	t.Run("recovers after transient failures", func(t *testing.T) {
		withNoSleep(t)
		argsFile := setUpFakePathBinaryFailingNTimes(t, goModDownloadMaxAttempts-1)

		require.NoError(t, runGoModDownload(t.TempDir(), nil))

		assert.Equal(t, goModDownloadMaxAttempts, readFakeBinInvocationCount(t, argsFile))
	})

	t.Run("fails after exhausting all attempts", func(t *testing.T) {
		withNoSleep(t)
		argsFile := setUpFakePathBinaryFailingNTimes(t, goModDownloadMaxAttempts+5)

		err := runGoModDownload(t.TempDir(), nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed after 3 attempts")
		assert.Equal(t, goModDownloadMaxAttempts, readFakeBinInvocationCount(t, argsFile),
			"must not retry beyond goModDownloadMaxAttempts")
	})
}
