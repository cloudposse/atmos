package cast

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	flagspkg "github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestRecordFlagsRegisteredThroughParser(t *testing.T) {
	if recordParser == nil {
		t.Fatal("expected record parser")
	}
	for _, name := range []string{recordFlagOutput, recordFlagFormat, recordFlagMode} {
		if recordParser.Registry().Get(name) == nil {
			t.Fatalf("expected record parser registry to include %q", name)
		}
		if recordCmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected record command to include %q flag", name)
		}
	}
}

func TestIsRecordCastTarget(t *testing.T) {
	tests := []struct {
		name   string
		output string
		format string
		want   bool
	}{
		{name: "cast extension", output: "out.cast", want: true},
		{name: "gif extension", output: "out.gif", want: false},
		{name: "explicit cast format wins over extension", output: "out.mp4", format: "cast", want: true},
		{name: "explicit non-cast format", output: "out.cast", format: "gif", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRecordCastTarget(tt.output, tt.format); got != tt.want {
				t.Fatalf("isRecordCastTarget(%q, %q) = %v, want %v", tt.output, tt.format, got, tt.want)
			}
		})
	}
}

func TestPlanRecordOutputCastTargetIsKeptDirectlyWithNoRenderPass(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo.cast")
	plan, err := planRecordOutput(output, "")
	if err != nil {
		t.Fatalf("planRecordOutput error: %v", err)
	}
	defer plan.Cleanup()
	if plan.CastPath != output {
		t.Fatalf("castPath = %q, want %q", plan.CastPath, output)
	}
	if plan.RenderOpts != nil {
		t.Fatalf("expected no render pass for a .cast target, got %+v", plan.RenderOpts)
	}
}

func TestPlanRecordOutputRenderedTargetUsesTempCastAndCleansUp(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo.mp4")
	plan, err := planRecordOutput(output, "")
	if err != nil {
		t.Fatalf("planRecordOutput error: %v", err)
	}
	if plan.CastPath == output {
		t.Fatal("expected a temp .cast path distinct from the render output")
	}
	if filepath.Ext(plan.CastPath) != ".cast" {
		t.Fatalf("temp path = %q, want a .cast extension", plan.CastPath)
	}
	if plan.RenderOpts == nil || plan.RenderOpts.MP4 != output {
		t.Fatalf("expected RenderOptions.MP4 = %q, got %+v", output, plan.RenderOpts)
	}
	if err := os.WriteFile(plan.CastPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan.Cleanup()
	if _, err := os.Stat(plan.CastPath); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove the temp cast file, stat err = %v", err)
	}
}

func TestPlanRecordOutputUnsupportedExtension(t *testing.T) {
	plan, err := planRecordOutput(filepath.Join(t.TempDir(), "demo.webm"), "")
	if plan.Cleanup != nil {
		defer plan.Cleanup()
	}
	if err == nil {
		t.Fatal("expected an error for an unsupported render extension")
	}
}

func resetRecordCommand(t *testing.T) {
	t.Helper()
	flagspkg.ResetCommandFlags(recordCmd)
}

// clearRecordViperOverrides removes any explicit viper.Set() override left by
// another test, using the prefixed keys the record parser's WithViperPrefix
// produces (cast.record.output, ...) -- distinct from render's bare keys, so
// the two commands' flag state can never collide.
func clearRecordViperOverrides(t *testing.T) {
	t.Helper()
	v := viper.GetViper()
	keys := []string{
		recordViperPrefix + "." + recordFlagOutput,
		recordViperPrefix + "." + recordFlagFormat,
		recordViperPrefix + "." + recordFlagMode,
	}
	for _, key := range keys {
		v.Set(key, nil)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			v.Set(key, nil)
		}
	})
}

func TestRecordCmdRunEReturnsErrorForMissingOutput(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	tapePath := filepath.Join(t.TempDir(), "demo.tape")
	if err := os.WriteFile(tapePath, []byte("Sleep 1ms"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := recordCmd.RunE(recordCmd, []string{tapePath})
	if !errors.Is(err, errUtils.ErrMissingRenderOutput) {
		t.Fatalf("expected ErrMissingRenderOutput, got %v", err)
	}
}

func TestRecordCmdRunEExecutesTapeAndRecordsCast(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	tapePath := filepath.Join(t.TempDir(), "demo.tape")
	if err := os.WriteFile(tapePath, []byte("Sleep 1ms"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "out.cast")
	if err := recordCmd.Flags().Set(recordFlagOutput, output); err != nil {
		t.Fatal(err)
	}
	recordCmd.SetContext(context.Background())

	if err := recordCmd.RunE(recordCmd, []string{tapePath}); err != nil {
		t.Fatalf("record command failed: %v", err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Size() == 0 {
		t.Fatalf("recorded cast missing or empty: %v", err)
	}
}

func TestRecordCmdRunEReturnsErrorForMissingTapeFile(t *testing.T) {
	resetRecordCommand(t)
	clearRecordViperOverrides(t)
	output := filepath.Join(t.TempDir(), "out.cast")
	if err := recordCmd.Flags().Set(recordFlagOutput, output); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing.tape")
	recordCmd.SetContext(context.Background())

	if err := recordCmd.RunE(recordCmd, []string{missing}); err == nil {
		t.Fatal("expected an error for a missing tape file")
	}
}
