package asciicast

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func execHelperEnv(t *testing.T, mode string) []string {
	t.Helper()
	return append(os.Environ(), asciicastExecHelperEnv+"="+mode)
}

func TestExecRecordCapturesOutputStreams(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	castPath := filepath.Join(t.TempDir(), "exec.cast")
	result, err := ExecRecord(context.Background(), &ExecOptions{
		Command: []string{exe},
		Env:     execHelperEnv(t, "ok"),
		Path:    castPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}

	_, events, err := ReadEvents(castPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr string
	for _, event := range events {
		switch event.Stream {
		case "o":
			stdout += event.Data
		case "e":
			stderr += event.Data
		}
	}
	if !strings.Contains(stdout, "stdout line") {
		t.Fatalf("stdout events = %q", stdout)
	}
	if !strings.Contains(stderr, "stderr line") {
		t.Fatalf("stderr events = %q", stderr)
	}
}

func TestExecRecordReturnsExitCodeWithoutError(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	castPath := filepath.Join(t.TempDir(), "exec-fail.cast")
	result, err := ExecRecord(context.Background(), &ExecOptions{
		Command: []string{exe},
		Env:     execHelperEnv(t, "fail"),
		Path:    castPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.ExitCode)
	}
	// Output before the failure must still be recorded.
	_, events, err := ReadEvents(castPath)
	if err != nil {
		t.Fatal(err)
	}
	var all string
	for _, event := range events {
		all += event.Data
	}
	if !strings.Contains(all, "about to fail") {
		t.Fatalf("events = %q", all)
	}
}

func TestExecRecordRequiresCommand(t *testing.T) {
	if _, err := ExecRecord(context.Background(), &ExecOptions{}); !errors.Is(err, ErrMissingExecCommand) {
		t.Fatalf("expected missing command error, got %v", err)
	}
	if _, err := ExecRecord(context.Background(), nil); !errors.Is(err, ErrMissingExecCommand) {
		t.Fatalf("expected missing command error for nil opts, got %v", err)
	}
}

func TestExecRecordPropagatesStartupErrors(t *testing.T) {
	_, err := ExecRecord(context.Background(), &ExecOptions{
		Command: []string{filepath.Join(t.TempDir(), "does-not-exist")},
		Path:    filepath.Join(t.TempDir(), "missing.cast"),
	})
	if err == nil {
		t.Fatal("expected error for missing executable")
	}
}

func TestExecRecordTreatsCancellationAsError(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	result, err := ExecRecord(ctx, &ExecOptions{
		Command: []string{exe},
		Env:     execHelperEnv(t, "sleep"),
		Path:    filepath.Join(t.TempDir(), "exec-cancel.cast"),
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}
}
