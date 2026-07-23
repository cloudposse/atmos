package scanners

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// testExePath returns the running test binary, used as a cross-platform
// stand-in for arbitrary subprocess behavior (see TestMain).
func testExePath(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return exe
}

func TestValidate(t *testing.T) {
	t.Run("nil scan", func(t *testing.T) {
		err := validate(nil)
		require.ErrorIs(t, err, errUtils.ErrNilParam)
	})

	t.Run("empty command", func(t *testing.T) {
		err := validate(&Context{Name: "tflint"})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})

	t.Run("valid", func(t *testing.T) {
		require.NoError(t, validate(&Context{Command: "tflint"}))
	})
}

func TestRun_RejectsInvalidScan(t *testing.T) {
	out, err := Run(context.Background(), nil)
	require.Nil(t, out)
	require.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestRun_SuccessNoOutput(t *testing.T) {
	exe := testExePath(t)
	scan := &Context{
		Name:    "fake",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
	}
	out, err := Run(context.Background(), scan)
	require.NoError(t, err)
	assert.Nil(t, out.Artifact)
	assert.Equal(t, 0, scan.ExitCode)
	assert.NoError(t, scan.CommandError)
}

func TestRun_CapturesOutputFile(t *testing.T) {
	exe := testExePath(t)
	scan := &Context{
		Name:    "fake",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
		Format:  "sarif",
		Env: map[string]string{
			"_ATMOS_TEST_WRITE_OUTPUT": "1",
			"_ATMOS_TEST_OUTPUT_BODY":  "hello from tool",
		},
	}
	out, err := Run(context.Background(), scan)
	require.NoError(t, err)
	require.NotNil(t, out.Artifact)
	assert.Equal(t, "hello from tool", string(out.Artifact.Body))
	assert.Equal(t, "sarif", out.Artifact.Format)
	assert.Equal(t, "fake", out.Artifact.Metadata["kind"])
}

func TestRun_CaptureStdoutRedirectsToOutputFile(t *testing.T) {
	const body = `{"runs":[{"tool":{"driver":{"name":"tflint"}}}]}`
	exe := testExePath(t)
	scan := &Context{
		Name:          "tflint",
		Command:       exe,
		Args:          []string{"-test.run", "^$"},
		CaptureStdout: true,
		Env: map[string]string{
			"_ATMOS_TEST_ECHO_STDOUT": "1",
			"_ATMOS_TEST_STDOUT_BODY": body,
		},
	}
	out, err := Run(context.Background(), scan)
	require.NoError(t, err)
	require.NotNil(t, out.Artifact, "stdout should have been captured into the output file")
	assert.Equal(t, body, string(out.Artifact.Body))
}

func TestRun_NoCaptureStdoutLeavesOutputFileEmpty(t *testing.T) {
	const body = "should not be captured"
	exe := testExePath(t)
	scan := &Context{
		Name:    "fake",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
		// CaptureStdout intentionally left false.
		Env: map[string]string{
			"_ATMOS_TEST_ECHO_STDOUT": "1",
			"_ATMOS_TEST_STDOUT_BODY": body,
		},
	}
	out, err := Run(context.Background(), scan)
	require.NoError(t, err)
	assert.Nil(t, out.Artifact)
}

func TestRun_InvokesResultHandler(t *testing.T) {
	exe := testExePath(t)
	called := false
	scan := &Context{
		Name:    "fake",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
		ResultHandler: func(_ *Context) (*Summary, error) {
			called = true
			return &Summary{Kind: "fake", Status: StatusSuccess, Title: "ok"}, nil
		},
	}
	out, err := Run(context.Background(), scan)
	require.NoError(t, err)
	assert.True(t, called)
	require.NotNil(t, out.Summary)
	assert.Equal(t, "ok", out.Summary.Title)
}

func TestRun_OnFailureModes(t *testing.T) {
	exe := testExePath(t)

	tests := []struct {
		name      string
		onFailure string
		wantErr   bool
	}{
		{"fail propagates", OnFailureFail, true},
		{"warn does not propagate", OnFailureWarn, false},
		{"ignore does not propagate", OnFailureIgnore, false},
		{"empty defaults to warn", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scan := &Context{
				Name:      "fake",
				Command:   exe,
				Args:      []string{"-test.run", "^$"},
				OnFailure: tt.onFailure,
				Env:       map[string]string{"_ATMOS_TEST_EXIT_ONE": "1"},
			}
			_, err := Run(context.Background(), scan)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, 1, scan.ExitCode)
			assert.Error(t, scan.CommandError)
		})
	}
}

func TestRun_ExpandsAtmosVarsInArgsAndEnv(t *testing.T) {
	exe := testExePath(t)
	scan := &Context{
		Name:    "fake",
		Command: exe,
		Args:    []string{"-test.run", "^$", "--", "$ATMOS_COMPONENT_PATH"},
		Env: map[string]string{
			"_ATMOS_TEST_WRITE_OUTPUT": "1",
			"_ATMOS_TEST_OUTPUT_BODY":  "$ATMOS_STACK",
		},
		Info: &schema.ConfigAndStacksInfo{Stack: "dev-stack"},
	}
	out, err := Run(context.Background(), scan)
	require.NoError(t, err)
	require.NotNil(t, out.Artifact)
	// The `$ATMOS_STACK` token in the env value is expanded by the engine
	// before exec, so the subprocess writes back the expanded value.
	assert.Equal(t, "dev-stack", string(out.Artifact.Body))
}

func TestPrepareSubprocess_MissingCommandReturnsCommandNotFound(t *testing.T) {
	scan := &Context{
		Name:    "fake",
		Command: filepath.Join(t.TempDir(), "missing-tool"),
	}
	_, err := prepareSubprocess(scan, t.TempDir(), filepath.Join(t.TempDir(), "output"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
}

func TestExitCodeFromErr(t *testing.T) {
	assert.Equal(t, 0, exitCodeFromErr(nil))
	assert.Equal(t, 1, exitCodeFromErr(errUtils.ErrCommandNotFound))

	exe := testExePath(t)
	cmd := exec.Command(exe, "-test.run", "^$") // #nosec G204 -- test binary invoked with a fixed, non-attacker-controlled arg list.
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_EXIT_ONE=1")
	err := cmd.Run()
	require.Error(t, err)
	assert.Equal(t, 1, exitCodeFromErr(err))
}

func TestApplyOnFailure(t *testing.T) {
	baseErr := errUtils.ErrCommandNotFound

	tests := []struct {
		name      string
		onFailure string
		wantErr   bool
	}{
		{"fail propagates wrapped error", OnFailureFail, true},
		{"warn swallows", OnFailureWarn, false},
		{"ignore swallows", OnFailureIgnore, false},
		{"unknown mode defaults to warn", "bogus", false},
		{"empty mode defaults to warn", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scan := &Context{Name: "fake", OnFailure: tt.onFailure}
			err := applyOnFailure(scan, baseErr)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, baseErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBuildAtmosEnv(t *testing.T) {
	t.Run("nil scan still returns component path and file vars", func(t *testing.T) {
		env := BuildAtmosEnv(nil, "/tmp/out", "/tmp")
		assert.Equal(t, "/tmp/out", env["ATMOS_OUTPUT_FILE"])
		assert.Equal(t, "/tmp", env["ATMOS_OUTPUT_DIR"])
		assert.NotContains(t, env, "ATMOS_STACK")
	})

	t.Run("populates stack and component from info", func(t *testing.T) {
		scan := &Context{Info: &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"}}
		env := BuildAtmosEnv(scan, "/tmp/out", "/tmp")
		assert.Equal(t, "dev", env["ATMOS_STACK"])
		assert.Equal(t, "vpc", env["ATMOS_COMPONENT"])
	})

	t.Run("planfile is always empty for now", func(t *testing.T) {
		scan := &Context{}
		env := BuildAtmosEnv(scan, "/tmp/out", "/tmp")
		assert.NotContains(t, env, "ATMOS_PLANFILE")
	})
}

func TestExpandEnvVars(t *testing.T) {
	t.Setenv("ATMOS_TEST_RUNNER_VAR", "from-os-env")

	assert.Equal(t, "value", expandEnvVars("$KEY", map[string]string{"KEY": "value"}))
	assert.Equal(t, "from-os-env", expandEnvVars("$ATMOS_TEST_RUNNER_VAR", nil))
	assert.Equal(t, "literal", expandEnvVars("literal", nil))
}

func TestMergeEnv(t *testing.T) {
	base := []string{"HOME=/tmp"}
	out := mergeEnv(base, map[string]string{"A": "1"}, map[string]string{"B": "2"})
	assert.Contains(t, out, "HOME=/tmp")
	assert.Contains(t, out, "A=1")
	assert.Contains(t, out, "B=2")
	// Original base slice must not be mutated (result/src isolation).
	assert.Equal(t, []string{"HOME=/tmp"}, base)
}

func TestPrependToolchainPATH(t *testing.T) {
	sep := string(os.PathListSeparator)
	base := []string{"HOME=/tmp", "PATH=/usr/bin"}
	assert.Equal(t, []string{"HOME=/tmp", "PATH=/tools" + sep + "/usr/bin"}, prependToolchainPATH(base, "/tools"))
	assert.Equal(t, []string{"HOME=/tmp", "PATH=/usr/bin"}, prependToolchainPATH(base, ""))
	assert.Equal(t, []string{"HOME=/tmp", "PATH=/tools"}, prependToolchainPATH([]string{"HOME=/tmp"}, "/tools"))
}

func TestPathFromEnv(t *testing.T) {
	assert.Equal(t, "/usr/bin", pathFromEnv([]string{"HOME=/tmp", "PATH=/usr/bin"}))
	// Falls back to the process PATH when not present in the given slice.
	assert.NotEmpty(t, pathFromEnv([]string{"HOME=/tmp"}))
}

func TestPathExtFromEnv(t *testing.T) {
	assert.Equal(t, ".EXE;.BAT", pathExtFromEnv([]string{"PATHEXT=.EXE;.BAT"}))
	assert.Equal(t, "", pathExtFromEnv([]string{"HOME=/tmp"}))
}

func TestResolveBinaryOnPath_RejectsNonExecutableUnixFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable bits do not apply on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(path, []byte("not executable"), 0o600))

	_, err := resolveBinaryOnPath("tool", dir, "", "", runtime.GOOS)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
}

func TestResolveBinaryOnPath_UsesWindowsPATHEXTForToolchainPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trivy.EXE")
	require.NoError(t, os.WriteFile(path, []byte("exe"), 0o600))

	got, err := resolveBinaryOnPath("trivy", dir, "", ".EXE;.BAT", "windows")
	require.NoError(t, err)
	assert.Equal(t, path, got)
}

func TestResolveBinaryOnPath_AbsolutePathVariants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable bits do not apply on Windows")
	}
	dir := t.TempDir()
	exePath := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(exePath, []byte("#!/bin/sh\n"), 0o755))

	got, err := resolveBinaryOnPath(exePath, "", "", "", runtime.GOOS)
	require.NoError(t, err)
	assert.Equal(t, exePath, got)

	_, err = resolveBinaryOnPath(filepath.Join(dir, "missing"), "", "", "", runtime.GOOS)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
}

func TestResolveBinaryOnPath_ErrorsOnEmptyAndMissingCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"empty command", ""},
		{"missing command", "definitely-not-on-path-atmos-scanners-test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveBinaryOnPath(tt.cmd, "", "", "", runtime.GOOS)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
		})
	}
}

func TestCombineSearchPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	assert.Equal(t, "toolchain", combineSearchPath("toolchain", ""))
	assert.Equal(t, "process", combineSearchPath("", "process"))
	assert.Equal(t, "toolchain"+sep+"process", combineSearchPath("toolchain", "process"))
}

func TestCandidateBinaryNames(t *testing.T) {
	assert.Equal(
		t,
		[]string{"trivy", "trivy.com", "trivy.exe", "trivy.bat", "trivy.cmd"},
		candidateBinaryNames("trivy", "", "windows"),
	)
	assert.Equal(
		t,
		[]string{"trivy", "trivy.EXE", "trivy.BAT"},
		candidateBinaryNames("trivy", "EXE;.EXE;.BAT", "windows"),
	)
	assert.Equal(t, []string{"trivy.exe"}, candidateBinaryNames("trivy.exe", ".COM;.EXE", "windows"))
	assert.Equal(t, []string{"trivy"}, candidateBinaryNames("trivy", ".EXE", "linux"))
}

func TestRenderTerminal(t *testing.T) {
	// None of these must panic; they exercise the guard clauses.
	renderTerminal(&Context{}, nil)
	renderTerminal(&Context{}, &Output{})
	renderTerminal(&Context{}, &Output{Summary: &Summary{TerminalBody: "plain source excerpt", Body: "**markdown fallback**"}})
	renderTerminal(&Context{}, &Output{Summary: &Summary{Body: "**hi**"}})
	renderTerminal(&Context{Format: FormatMarkdown}, &Output{Artifact: &Artifact{Body: []byte("**hi**")}})
	// Non-markdown artifact format must not attempt to render.
	renderTerminal(&Context{Format: "sarif"}, &Output{Artifact: &Artifact{Body: []byte("{}")}})
}
