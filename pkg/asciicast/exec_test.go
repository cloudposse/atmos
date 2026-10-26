package asciicast

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/process"
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

func TestExecRecordPropagatesRecorderStartError(t *testing.T) {
	// ExecOptions has no Overwrite/Explicit knob, so Start refuses to record
	// over a path that already exists, exercising the `if err != nil` branch
	// right after Start in ExecRecord.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "exists.cast")
	if err := os.WriteFile(path, []byte("already here"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ExecRecord(context.Background(), &ExecOptions{
		Command: []string{exe},
		Env:     execHelperEnv(t, "ok"),
		Path:    path,
	})
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !errors.Is(err, ErrCastOutputExists) {
		t.Fatalf("expected cast output exists error, got %v", err)
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

func TestStreamWriterPropagatesEventError(t *testing.T) {
	// A Recorder backed by a broken writer (with a buffer too small to
	// absorb the marshaled event) makes rec.Event fail without any OS-level
	// trick, exercising streamWriter.Write's error-wrapping branch.
	rec := &Recorder{writer: bufio.NewWriterSize(&errWriter{failAfter: 0}, 1)}

	w := &streamWriter{rec: rec, stream: "o"}
	n, err := w.Write([]byte("output"))
	if n != 0 {
		t.Fatalf("n = %d, want 0 on error", n)
	}
	if err == nil || !strings.Contains(err.Error(), "record o event") {
		t.Fatalf("expected wrapped record event error, got %v", err)
	}
}

func TestExecRecordResultJoinsCloseAndResultErrors(t *testing.T) {
	closeErr := errors.New("close failed")
	resultErr := errors.New("process failed")

	result, err := execRecordResult(&process.Result{Err: resultErr}, closeErr)
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !errors.Is(err, closeErr) || !errors.Is(err, resultErr) {
		t.Fatalf("expected joined close and result errors, got %v", err)
	}
}

func TestExecRecordResultReturnsCloseErrAloneWhenResultSucceeded(t *testing.T) {
	closeErr := errors.New("close failed")

	result, err := execRecordResult(&process.Result{}, closeErr)
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("expected close error alone, got %v", err)
	}
}

func TestExecRecordResultReturnsCanceledErrorWithoutExitCode(t *testing.T) {
	cancelErr := errors.New("context canceled")

	result, err := execRecordResult(&process.Result{Err: cancelErr, Canceled: true, ExitCode: 0}, nil)
	if result != nil {
		t.Fatalf("result = %#v, want nil for canceled result", result)
	}
	if !errors.Is(err, cancelErr) {
		t.Fatalf("expected canceled error, got %v", err)
	}
}

func TestExecRecordResultReturnsResultErrWhenExitCodeNegative(t *testing.T) {
	startErr := errors.New("failed to start")

	result, err := execRecordResult(&process.Result{Err: startErr, ExitCode: -1}, nil)
	if result != nil {
		t.Fatalf("result = %#v, want nil when exit code is negative", result)
	}
	if !errors.Is(err, startErr) {
		t.Fatalf("expected start error, got %v", err)
	}
}

func TestExecRecordResultSucceedsWithNoErrors(t *testing.T) {
	result, err := execRecordResult(&process.Result{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.ExitCode != 0 {
		t.Fatalf("result = %#v, want ExitCode 0", result)
	}
}
