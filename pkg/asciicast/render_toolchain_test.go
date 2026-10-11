package asciicast

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TestRunRendererRejectsEmptyBinary covers the guard clause in runRenderer
// that refuses to exec.Command with an empty path. This protects against a
// resolveRenderTools bug that returns a renderTools with an unset field
// (e.g. tools.ffmpeg left empty) reaching exec.Command, which would otherwise
// surface as a confusing "file not found" error instead of a clear signal
// that the managed renderer path was never resolved.
func TestRunRendererRejectsEmptyBinary(t *testing.T) {
	err := runRenderer("")
	if !errors.Is(err, errUtils.ErrToolInstall) {
		t.Fatalf("expected wrapped ErrToolInstall for empty binary, got %v", err)
	}
}

// TestRunRendererWrapsExecutionFailure asserts that a failing renderer
// process surfaces as the dedicated errUtils.ErrRenderToolExecFailed
// sentinel, with the binary path folded into the message, so callers can use
// errors.Is to distinguish "renderer process failed" from other failure
// modes (e.g. errUtils.ErrToolInstall for a renderer that couldn't be
// resolved/installed at all).
func TestRunRendererWrapsExecutionFailure(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(asciicastExecHelperEnv, "fail")

	runErr := runRenderer(exe)
	if runErr == nil {
		t.Fatal("expected error from failing renderer process")
	}
	if !errors.Is(runErr, errUtils.ErrRenderToolExecFailed) {
		t.Fatalf("expected ErrRenderToolExecFailed, got %v", runErr)
	}
	if !strings.Contains(runErr.Error(), exe) {
		t.Fatalf("expected error to name the failed binary %q, got %v", exe, runErr)
	}
}

// TestRunRendererSucceedsWithoutError confirms the happy path returns nil
// once the subprocess exits cleanly, exercising the fall-through after
// cmd.Run() succeeds (the counterpart to the failure-wrapping test above).
func TestRunRendererSucceedsWithoutError(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(asciicastExecHelperEnv, "ok")

	if err := runRenderer(exe); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

// TestRunRendererRoutesOutputThroughIOMasking proves cmd.Stdout/cmd.Stderr
// are wired to the Atmos IO layer's Data()/UI() writers (which apply
// masking) rather than directly to os.Stdout/os.Stderr. A regression back to
// raw os.Stdout/os.Stderr would still forward the bytes through (the IO
// layer's dynamic writers resolve os.Stdout/os.Stderr at write time
// specifically to support this kind of test capture) but would silently stop
// masking secrets a renderer subprocess happens to echo. Registering the
// helper's fixed output strings as secrets and asserting they come out
// replaced with the mask marker is what actually distinguishes the two
// implementations.
func TestRunRendererRoutesOutputThroughIOMasking(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(asciicastExecHelperEnv, "ok")

	iolib.RegisterSecret("stdout line")
	iolib.RegisterSecret("stderr line")

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutW, stderrW
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origStdout, origStderr
	})

	runErr := runRenderer(exe)

	_ = stdoutW.Close()
	_ = stderrW.Close()
	stdoutBytes, readErr := io.ReadAll(stdoutR)
	if readErr != nil {
		t.Fatal(readErr)
	}
	stderrBytes, readErr := io.ReadAll(stderrR)
	if readErr != nil {
		t.Fatal(readErr)
	}

	if runErr != nil {
		t.Fatalf("runRenderer: %v", runErr)
	}
	if strings.Contains(string(stdoutBytes), "stdout line") {
		t.Fatalf("expected masked stdout (raw secret leaked), got %q", stdoutBytes)
	}
	if !strings.Contains(string(stdoutBytes), iolib.MaskReplacement) {
		t.Fatalf("expected stdout to contain the mask replacement, got %q", stdoutBytes)
	}
	if strings.Contains(string(stderrBytes), "stderr line") {
		t.Fatalf("expected masked stderr (raw secret leaked), got %q", stderrBytes)
	}
	if !strings.Contains(string(stderrBytes), iolib.MaskReplacement) {
		t.Fatalf("expected stderr to contain the mask replacement, got %q", stderrBytes)
	}
}

// withRenderToolInstallPolicy overrides whether toolchain.install permits downloading the
// managed renderers.
func withRenderToolInstallPolicy(t *testing.T, allowed bool, err error) {
	t.Helper()
	previous := renderToolInstallAllowed
	renderToolInstallAllowed = func() (bool, error) { return allowed, err }
	t.Cleanup(func() { renderToolInstallAllowed = previous })
}

func TestResolveRenderToolsSkipsInstallWhenPolicyIsNever(t *testing.T) {
	withRenderToolInstallPolicy(t, false, nil)
	var located []string
	withRenderToolchainHooks(
		t,
		func(map[string]string) error {
			t.Fatal("toolchain.install=never must not install the managed renderers")
			return nil
		},
		func(spec renderToolSpec) (string, error) {
			located = append(located, spec.binary)
			return filepath.Join(t.TempDir(), spec.binary), nil
		},
	)

	tools, err := resolveRenderToolsFromToolchain(renderToolRequirements{agg: true, ffmpeg: true})
	if err != nil {
		t.Fatal(err)
	}
	if tools.agg == "" || tools.ffmpeg == "" {
		t.Fatalf("installed renderers must still resolve: %#v", tools)
	}
	if strings.Join(located, ",") != "agg,ffmpeg" {
		t.Fatalf("located %v, want agg and ffmpeg", located)
	}
}

func TestResolveRenderToolsReportsMissingRendererWhenPolicyIsNever(t *testing.T) {
	withRenderToolInstallPolicy(t, false, nil)
	withRenderToolchainHooks(
		t,
		func(map[string]string) error {
			t.Fatal("toolchain.install=never must not install the managed renderers")
			return nil
		},
		func(renderToolSpec) (string, error) { return "", errors.New("not found") },
	)

	_, err := resolveRenderToolsFromToolchain(renderToolRequirements{agg: true})

	if !errors.Is(err, errUtils.ErrToolNotInstalled) {
		t.Fatalf("expected ErrToolNotInstalled, got %v", err)
	}
	if errors.Is(err, errUtils.ErrToolInstall) {
		t.Fatalf("a forbidden install is not an install failure: %v", err)
	}
	formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	if !strings.Contains(formatted, "atmos toolchain install "+aggTool+"@"+aggVersion) {
		t.Fatalf("expected an install hint naming the renderer, got %s", formatted)
	}
}

func TestResolveRenderToolsInstallsWhenPolicyAllows(t *testing.T) {
	withRenderToolInstallPolicy(t, true, nil)
	installed := false
	withRenderToolchainHooks(
		t,
		func(map[string]string) error { installed = true; return nil },
		func(spec renderToolSpec) (string, error) { return filepath.Join(t.TempDir(), spec.binary), nil },
	)

	if _, err := resolveRenderToolsFromToolchain(renderToolRequirements{agg: true}); err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("an allowed policy must install the managed renderers")
	}
}

func TestResolveRenderToolsPropagatesInvalidPolicy(t *testing.T) {
	want := errors.New("invalid policy")
	withRenderToolInstallPolicy(t, false, want)
	withRenderToolchainHooks(
		t,
		func(map[string]string) error { t.Fatal("must not install"); return nil },
		func(renderToolSpec) (string, error) { t.Fatal("must not locate"); return "", nil },
	)

	_, err := resolveRenderToolsFromToolchain(renderToolRequirements{agg: true})

	if !errors.Is(err, want) {
		t.Fatalf("expected the policy error, got %v", err)
	}
}

func TestRenderToolInstallAllowedReadsToolchainConfig(t *testing.T) {
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })

	for policy, want := range map[schema.ToolchainInstall]bool{
		"":                              true, // An unset policy is auto, which installs.
		schema.ToolchainInstallNever:    false,
		schema.ToolchainInstallDeclared: true,
		schema.ToolchainInstallAuto:     true,
		schema.ToolchainInstallAlways:   true,
	} {
		toolchain.SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{Install: policy}})
		got, err := renderToolInstallAllowed()
		if err != nil || got != want {
			t.Fatalf("policy %q: got (%v, %v), want (%v, nil)", policy, got, err, want)
		}
	}
}
