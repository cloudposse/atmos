package cast

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestPlanRecordOutputErrors(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		format  string
		wantErr error
	}{
		{name: "unsupported extension", output: "demo.webm", wantErr: errUtils.ErrUnsupportedCastOutputExtension},
		{name: "format conflicts with extension", output: "demo.gif", format: "mp4", wantErr: errUtils.ErrInvalidFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := planRecordOutput(filepath.Join(t.TempDir(), tt.output), tt.format)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if plan.RenderOpts != nil || plan.CastPath != "" {
				t.Fatalf("expected an empty plan on error, got %+v", plan)
			}
		})
	}
}

// TestPlanRecordOutputFailsWhenTempDirectoryIsUnusable proves a rendered
// target surfaces the temp-file creation failure instead of recording to an
// unwritable location.
func TestPlanRecordOutputFailsWhenTempDirectoryIsUnusable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does", "not", "exist")
	t.Setenv("TMPDIR", missing)
	t.Setenv("TMP", missing)
	t.Setenv("TEMP", missing)

	plan, err := planRecordOutput(filepath.Join(t.TempDir(), "demo.gif"), "")
	if err == nil {
		plan.Cleanup()
		t.Fatal("expected an error when the temp directory does not exist")
	}
	if !errors.Is(err, errUtils.ErrCreateTempDir) {
		t.Fatalf("expected ErrCreateTempDir, got %v", err)
	}
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected the underlying *fs.PathError to stay in the chain, got %v", err)
	}
	if plan.RenderOpts != nil {
		t.Fatalf("expected an empty plan on error, got %+v", plan)
	}
}

func TestRecordCmdRunEReturnsPlanErrorForUnsupportedOutput(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	tapePath := filepath.Join(t.TempDir(), "demo.tape")
	if err := os.WriteFile(tapePath, []byte("Sleep 1ms"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := recordCmd.Flags().Set(recordFlagOutput, filepath.Join(t.TempDir(), "out.webm")); err != nil {
		t.Fatal(err)
	}
	recordCmd.SetContext(context.Background())

	err := recordCmd.RunE(recordCmd, []string{tapePath})
	if !errors.Is(err, errUtils.ErrUnsupportedCastOutputExtension) {
		t.Fatalf("expected ErrUnsupportedCastOutputExtension, got %v", err)
	}
}

// TestRecordCmdRunERendersRecordingToRequestedFormat covers the render pass:
// a non-.cast output records to a temp cast, renders it to the requested
// artifact, and removes the intermediate recording.
func TestRecordCmdRunERendersRecordingToRequestedFormat(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	data.InitWriter(iolib.GetContext())
	ui.InitFormatter(iolib.GetContext())

	tapePath := filepath.Join(t.TempDir(), "demo.tape")
	if err := os.WriteFile(tapePath, []byte(`Type "echo rendered-by-record" Enter`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "out.html")
	if err := recordCmd.Flags().Set(recordFlagOutput, output); err != nil {
		t.Fatal(err)
	}
	if err := recordCmd.Flags().Set(recordFlagMode, recordModeSteps); err != nil {
		t.Fatal(err)
	}
	recordCmd.SetContext(context.Background())

	if err := recordCmd.RunE(recordCmd, []string{tapePath}); err != nil {
		t.Fatalf("record command failed: %v", err)
	}
	rendered, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("rendered output missing: %v", err)
	}
	if !strings.Contains(string(rendered), "rendered-by-record") {
		t.Fatalf("rendered HTML does not contain the recorded command output:\n%s", rendered)
	}
}

// TestRecordCmdRunEReturnsRenderError proves a render failure surfaces to the
// caller (and the temp recording is still cleaned up by the plan) instead of
// being reported as a successful render.
func TestRecordCmdRunEReturnsRenderError(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	data.InitWriter(iolib.GetContext())
	ui.InitFormatter(iolib.GetContext())

	tapePath := filepath.Join(t.TempDir(), "demo.tape")
	if err := os.WriteFile(tapePath, []byte(`Type "echo hi" Enter`), 0o644); err != nil {
		t.Fatal(err)
	}
	// A regular file where the output's parent directory should be makes the
	// render step unable to write its artifact.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(blocker, "out.html")
	if err := recordCmd.Flags().Set(recordFlagOutput, output); err != nil {
		t.Fatal(err)
	}
	if err := recordCmd.Flags().Set(recordFlagMode, recordModeSteps); err != nil {
		t.Fatal(err)
	}
	recordCmd.SetContext(context.Background())

	// The render pass cannot create a parent directory beneath a regular
	// file; the OS path error must reach the caller unmasked.
	err := recordCmd.RunE(recordCmd, []string{tapePath})
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("expected a *fs.PathError render error when the output location is not writable, got %v", err)
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatalf("no artifact should exist at %s after a failed render", output)
	}
}

func TestCheckRecordFormatMatchesExtension(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		format  string
		wantErr error
	}{
		{name: "no format", output: "x.gif"},
		{name: "matching cast", output: "x.cast", format: "cast"},
		{name: "matching gif", output: "x.gif", format: "gif"},
		{name: "matching case-insensitive", output: "x.GIF", format: "gif"},
		{name: "jpg extension with jpeg format", output: "x.jpg", format: "jpeg"},
		{name: "jpeg extension with jpg format", output: "x.jpeg", format: "jpg"},
		{name: "extensionless output accepts cast", output: "x", format: "cast"},
		{name: "extensionless output accepts gif", output: "x", format: "gif"},
		{name: "unrecognized extension accepts any format", output: "x.txt", format: "gif"},
		{name: "gif extension with cast format", output: "x.gif", format: "cast", wantErr: errUtils.ErrInvalidFlag},
		{name: "cast extension with gif format", output: "y.cast", format: "gif", wantErr: errUtils.ErrInvalidFlag},
		{name: "gif extension with html format", output: "z.gif", format: "html", wantErr: errUtils.ErrInvalidFlag},
		{name: "unsupported format", output: "x.gif", format: "webm", wantErr: errUtils.ErrInvalidFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRecordFormatMatchesExtension(tt.output, tt.format)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestPlanRecordOutputRejectsSymmetricFormatExtensionConflicts(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct{ output, format string }{
		{output: "x.gif", format: "cast"},
		{output: "y.cast", format: "gif"},
	} {
		plan, err := planRecordOutput(filepath.Join(dir, tt.output), tt.format)
		if !errors.Is(err, errUtils.ErrInvalidFlag) {
			t.Fatalf("planRecordOutput(%q, %q): expected ErrInvalidFlag, got %v", tt.output, tt.format, err)
		}
		if !strings.Contains(err.Error(), "conflicts with output extension") {
			t.Fatalf("expected the conflict wording, got %q", err.Error())
		}
		if plan.CastPath != "" || plan.RenderOpts != nil {
			t.Fatalf("expected an empty plan on error, got %+v", plan)
		}
	}
}

func TestPlanRecordOutputExtensionlessOutputHonorsFormat(t *testing.T) {
	out := filepath.Join(t.TempDir(), "demo")

	castPlan, err := planRecordOutput(out, "cast")
	if err != nil {
		t.Fatalf("cast format: %v", err)
	}
	defer castPlan.Cleanup()
	if castPlan.CastPath != out || castPlan.RenderOpts != nil {
		t.Fatalf("expected the raw recording kept at %q, got %+v", out, castPlan)
	}

	gifPlan, err := planRecordOutput(out, "gif")
	if err != nil {
		t.Fatalf("gif format: %v", err)
	}
	defer gifPlan.Cleanup()
	if gifPlan.RenderOpts == nil || gifPlan.RenderOpts.GIF != out {
		t.Fatalf("expected a GIF render to %q, got %+v", out, gifPlan.RenderOpts)
	}
}

func TestPlanRecordOutputRenderedTargetOverwritesExistingOutput(t *testing.T) {
	plan, err := planRecordOutput(filepath.Join(t.TempDir(), "demo.gif"), "")
	if err != nil {
		t.Fatalf("planRecordOutput error: %v", err)
	}
	defer plan.Cleanup()
	if plan.RenderOpts == nil || !plan.RenderOpts.Overwrite {
		t.Fatalf("expected RenderOptions.Overwrite for an explicit --output, got %+v", plan.RenderOpts)
	}
}

func TestBuildRecordStepCarriesShellInit(t *testing.T) {
	step := buildRecordStep("demo.tape", recordModeSession, recordShellOptions{Shell: "bash -o vi", ShellInit: "user"}, &recordOutputPlan{CastPath: "out.cast"})
	if step.ShellInit != "user" || step.Shell != "bash -o vi" || step.Mode != recordModeSession || step.TapeFile != "demo.tape" {
		t.Fatalf("unexpected step: %+v", step)
	}
	if step.CastOutput == nil || step.CastOutput.Cast != "out.cast" {
		t.Fatalf("expected the cast output path to be set, got %+v", step.CastOutput)
	}
	if step.CastOutput.Intermediate {
		t.Fatalf("a raw .cast target is the user's output and must be announced, got %+v", step.CastOutput)
	}
	rendered := buildRecordStep("demo.tape", recordModeSession, recordShellOptions{}, &recordOutputPlan{CastPath: "tmp.cast", RenderOpts: &asciicast.RenderOptions{GIF: "out.gif"}})
	if !rendered.CastOutput.Intermediate {
		t.Fatalf("a temporary .cast feeding a render must be marked intermediate, got %+v", rendered.CastOutput)
	}

	if unset := buildRecordStep("demo.tape", recordModeSteps, recordShellOptions{}, &recordOutputPlan{CastPath: "out.cast"}); unset.ShellInit != "" || unset.Shell != "" {
		t.Fatalf("an unset flag must leave ShellInit empty so lower-precedence defaults apply, got %q", unset.ShellInit)
	}
}

func TestRecordShellInitReadsEnvironmentVariable(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	t.Setenv("ATMOS_CAST_RECORD_SHELL_INIT", "user")

	if err := recordParser.BindFlagsToViper(recordCmd, viper.GetViper()); err != nil {
		t.Fatalf("bind flags: %v", err)
	}
	if got := viper.GetString(recordViperPrefix + "." + recordFlagShellInit); got != "user" {
		t.Fatalf("shell-init = %q, want %q from ATMOS_CAST_RECORD_SHELL_INIT", got, "user")
	}
}

func TestRecordShellReadsEnvironmentVariable(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	t.Setenv("ATMOS_CAST_RECORD_SHELL", "bash -o vi")

	if err := recordParser.BindFlagsToViper(recordCmd, viper.GetViper()); err != nil {
		t.Fatalf("bind flags: %v", err)
	}
	if got := viper.GetString(recordViperPrefix + "." + recordFlagShell); got != "bash -o vi" {
		t.Fatalf("shell = %q, want %q from ATMOS_CAST_RECORD_SHELL", got, "bash -o vi")
	}
}

func TestRecordCmdRunERejectsInvalidModeBeforeRecording(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	tapePath := filepath.Join(t.TempDir(), "demo.tape")
	if err := os.WriteFile(tapePath, []byte("Sleep 1ms"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "out.cast")
	if err := recordCmd.Flags().Set(recordFlagOutput, output); err != nil {
		t.Fatal(err)
	}
	if err := recordCmd.Flags().Set(recordFlagMode, "bogus"); err != nil {
		t.Fatal(err)
	}
	recordCmd.SetContext(context.Background())

	err := recordCmd.RunE(recordCmd, []string{tapePath})
	if !errors.Is(err, errUtils.ErrInvalidFlagValue) {
		t.Fatalf("expected ErrInvalidFlagValue, got %v", err)
	}
	if !strings.Contains(err.Error(), "session, steps") {
		t.Fatalf("expected the error to list valid modes, got %q", err.Error())
	}
	if _, statErr := os.Stat(output); statErr == nil {
		t.Fatalf("nothing should be recorded after a rejected --mode, found %s", output)
	}
}

func TestRecordCmdRunERejectsInvalidFormat(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	tapePath := filepath.Join(t.TempDir(), "demo.tape")
	if err := os.WriteFile(tapePath, []byte("Sleep 1ms"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := recordCmd.Flags().Set(recordFlagOutput, filepath.Join(t.TempDir(), "out")); err != nil {
		t.Fatal(err)
	}
	if err := recordCmd.Flags().Set(recordFlagFormat, "webm"); err != nil {
		t.Fatal(err)
	}
	recordCmd.SetContext(context.Background())

	if err := recordCmd.RunE(recordCmd, []string{tapePath}); !errors.Is(err, errUtils.ErrInvalidFlagValue) {
		t.Fatalf("expected ErrInvalidFlagValue, got %v", err)
	}
}

func TestRecordCmdRunERejectsFormatExtensionConflictBeforeRecording(t *testing.T) {
	for _, tt := range []struct{ output, format string }{
		{output: "x.gif", format: "cast"},
		{output: "y.cast", format: "gif"},
	} {
		t.Run(tt.output+" with "+tt.format, func(t *testing.T) {
			resetRecordCommand(t)
			clearRecordViperOverrides(t)
			tapePath := filepath.Join(t.TempDir(), "demo.tape")
			if err := os.WriteFile(tapePath, []byte("Sleep 1ms"), 0o644); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), tt.output)
			if err := recordCmd.Flags().Set(recordFlagOutput, output); err != nil {
				t.Fatal(err)
			}
			if err := recordCmd.Flags().Set(recordFlagFormat, tt.format); err != nil {
				t.Fatal(err)
			}
			recordCmd.SetContext(context.Background())

			if err := recordCmd.RunE(recordCmd, []string{tapePath}); !errors.Is(err, errUtils.ErrInvalidFlag) {
				t.Fatalf("expected ErrInvalidFlag, got %v", err)
			}
			if _, statErr := os.Stat(output); statErr == nil {
				t.Fatalf("nothing should be written after a rejected --format, found %s", output)
			}
		})
	}
}

// TestRecordCmdRunEOverwritesExistingRenderedOutput proves re-running record
// against the same --output replaces the artifact instead of failing with
// ErrRenderOutputExists after a full recording.
func TestRecordCmdRunEOverwritesExistingRenderedOutput(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	data.InitWriter(iolib.GetContext())
	ui.InitFormatter(iolib.GetContext())

	output := filepath.Join(t.TempDir(), "out.html")
	for _, marker := range []string{"first-recording-marker", "second-recording-marker"} {
		resetRecordCommand(t)
		clearRecordViperOverrides(t)
		tapePath := filepath.Join(t.TempDir(), "demo.tape")
		if err := os.WriteFile(tapePath, []byte(`Type "echo `+marker+`" Enter`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := recordCmd.Flags().Set(recordFlagOutput, output); err != nil {
			t.Fatal(err)
		}
		if err := recordCmd.Flags().Set(recordFlagMode, recordModeSteps); err != nil {
			t.Fatal(err)
		}
		recordCmd.SetContext(context.Background())

		if err := recordCmd.RunE(recordCmd, []string{tapePath}); err != nil {
			t.Fatalf("record with marker %q failed: %v", marker, err)
		}
		rendered, err := os.ReadFile(output)
		if err != nil {
			t.Fatalf("rendered output missing: %v", err)
		}
		if !strings.Contains(string(rendered), marker) {
			t.Fatalf("rendered output does not contain %q", marker)
		}
		if marker == "second-recording-marker" && strings.Contains(string(rendered), "first-recording-marker") {
			t.Fatal("second run must replace the first run's output")
		}
	}
}
