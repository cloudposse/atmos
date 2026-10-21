package cast

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
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

	if err := recordCmd.RunE(recordCmd, []string{tapePath}); err == nil {
		t.Fatal("expected a render error when the output location is not writable")
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatalf("no artifact should exist at %s after a failed render", output)
	}
}
