package pty

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

func ptyHelperCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	cmd := exec.Command(exe, append([]string{"-test.run=TestPTYHelper", "--"}, args...)...)
	// The test binary links TUI libraries that query the terminal (OSC/DSR)
	// when stdout is a TTY and block waiting for replies no test PTY will
	// send. A dumb terminal suppresses the queries.
	cmd.Env = append(os.Environ(), "TERM=dumb", "NO_COLOR=1")
	return cmd
}

func TestPTYHelper(t *testing.T) {
	args := ptyHelperArgs()
	if args == nil {
		return
	}

	switch args[0] {
	case "print-done":
		fmt.Println("done")
		time.Sleep(100 * time.Millisecond)
	case "stdin-marker":
		if isCharDevice(os.Stdin) {
			fmt.Println("is-a-tty")
		}
		if line, ok := readLineWithTimeout(300 * time.Millisecond); ok {
			fmt.Println("INPUT-RECEIVED:" + strings.TrimSpace(line))
		} else {
			fmt.Println("NO-INPUT")
		}
	default:
		t.Fatalf("unknown helper command: %s", args[0])
	}
	os.Exit(0)
}

func ptyHelperArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			return os.Args[i+1:]
		}
	}
	return nil
}

func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func readLineWithTimeout(timeout time.Duration) (string, bool) {
	lines := make(chan string, 1)
	go func() {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err == nil || line != "" {
			lines <- line
			return
		}
		lines <- ""
	}()

	select {
	case line := <-lines:
		return line, line != ""
	case <-time.After(timeout):
		return "", false
	}
}

func TestIsSupported(t *testing.T) {
	supported := IsSupported()

	switch runtime.GOOS {
	case "darwin", "linux":
		if !supported {
			t.Errorf("IsSupported() = false on %s, expected true", runtime.GOOS)
		}
	case "windows":
		if supported {
			t.Errorf("IsSupported() = true on windows, expected false")
		}
	default:
		// Other platforms should return false.
		if supported {
			t.Errorf("IsSupported() = true on %s, expected false", runtime.GOOS)
		}
	}
}

func TestExecWithPTY_UnsupportedPlatform(t *testing.T) {
	if IsSupported() {
		t.Skip("Skipping unsupported platform test on supported platform")
	}

	ctx := context.Background()
	cmd := exec.Command("echo", "test")
	opts := &Options{}

	err := ExecWithPTY(ctx, cmd, opts)
	if err == nil {
		t.Error("ExecWithPTY() on unsupported platform should return error")
	}

	if !strings.Contains(err.Error(), "PTY not supported") {
		t.Errorf("Expected 'PTY not supported' error, got: %v", err)
	}
}

func TestExecWithPTY_BasicExecution(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout bytes.Buffer

	// Subprocess writes output then sleeps briefly before exiting.
	// The sleep ensures the PTY has time to read all buffered output before
	// the subprocess exits and triggers EIO. This avoids the race condition
	// described in https://go.dev/issue/57141
	cmd := exec.Command("sh", "-c", "printf '%s\\n' 'hello world'; sleep 0.1")
	opts := &Options{
		Stdin:         strings.NewReader(""), // Provide empty stdin for CI environments.
		Stdout:        &stdout,
		EnableMasking: false,
	}

	err := ExecWithPTY(ctx, cmd, opts)
	if err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "hello world") {
		t.Errorf("Expected output to contain 'hello world', got: %s", output)
	}
}

func TestExecWithPTY_WithMasking(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create IO context with masking enabled.
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to create IO context: %v", err)
	}

	var stdout bytes.Buffer
	secretKey := "AKIAIOSFODNN7EXAMPLE"

	// Subprocess writes output then sleeps briefly before exiting.
	// The sleep ensures the PTY has time to read all buffered output before
	// the subprocess exits and triggers EIO. This avoids the race condition
	// described in https://go.dev/issue/57141
	cmd := exec.Command("sh", "-c", "printf '%s\\n' '"+secretKey+"'; sleep 0.1")

	opts := &Options{
		Stdin:         strings.NewReader(""),
		Stdout:        &stdout,
		Masker:        ioCtx.Masker(),
		EnableMasking: true,
	}

	err = ExecWithPTY(ctx, cmd, opts)
	if err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}

	output := stdout.String()

	// Output should NOT contain the actual secret.
	if strings.Contains(output, secretKey) {
		t.Errorf("Output contains unmasked secret: %s", output)
	}

	// Output should contain the mask replacement.
	if !strings.Contains(output, iolib.MaskReplacement) {
		t.Errorf("Output does not contain mask replacement '%s': %s", iolib.MaskReplacement, output)
	}
}

func TestExecWithPTY_MaskingDisabled(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create IO context.
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to create IO context: %v", err)
	}

	var stdout bytes.Buffer
	secretKey := "AKIAIOSFODNN7EXAMPLE"

	// Subprocess writes output then sleeps briefly before exiting.
	// The sleep ensures the PTY has time to read all buffered output before
	// the subprocess exits and triggers EIO. This avoids the race condition
	// described in https://go.dev/issue/57141
	cmd := exec.Command("sh", "-c", "printf '%s\\n' '"+secretKey+"'; sleep 0.1")

	opts := &Options{
		Stdin:         strings.NewReader(""),
		Stdout:        &stdout,
		Masker:        ioCtx.Masker(),
		EnableMasking: false, // Explicitly disabled.
	}

	err = ExecWithPTY(ctx, cmd, opts)
	if err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}

	output := stdout.String()

	// Output SHOULD contain the actual secret when masking is disabled.
	if !strings.Contains(output, secretKey) {
		t.Errorf("Output should contain unmasked secret when masking disabled, got: %q (bytes: %v)", output, []byte(output))
	}
}

func TestExecWithPTY_ContextCancellation(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var stdout bytes.Buffer

	// Sleep command that would run for 10 seconds.
	cmd := exec.Command("sleep", "10")
	opts := &Options{
		Stdin:  strings.NewReader(""), // Provide empty stdin for CI environments.
		Stdout: &stdout,
	}

	// Cancel context after 100ms.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := ExecWithPTY(ctx, cmd, opts)

	// Should return context.Canceled error.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}

func TestExecWithPTY_CommandFailure(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout bytes.Buffer

	// Command that will fail (exit code 1).
	cmd := exec.Command("sh", "-c", "exit 1")
	opts := &Options{
		Stdin:  strings.NewReader(""), // Provide empty stdin for CI environments.
		Stdout: &stdout,
	}

	err := ExecWithPTY(ctx, cmd, opts)

	// Should return non-nil error.
	if err == nil {
		t.Error("ExecWithPTY() with failing command should return error")
	}
}

func TestExecWithPTY_DefaultOptions(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use sh -c with printf and a small sleep to ensure the PTY has time to read
	// all buffered output before the subprocess exits and triggers EIO.
	// This avoids the race condition described in https://go.dev/issue/57141
	cmd := exec.Command("sh", "-c", "printf '%s\\n' 'test'; sleep 0.1")

	// Pass nil options - should use defaults.
	err := ExecWithPTY(ctx, cmd, nil)
	if err != nil {
		t.Errorf("ExecWithPTY() with nil options error = %v", err)
	}
}

func TestMaskedWriter_Write(t *testing.T) {
	// Create IO context with masking enabled.
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to create IO context: %v", err)
	}

	var buf bytes.Buffer

	writer := &maskedWriter{
		underlying: &buf,
		masker:     ioCtx.Masker(),
	}

	secretKey := "AKIAIOSFODNN7EXAMPLE"
	input := []byte("AWS key: " + secretKey + "\n")

	n, err := writer.Write(input)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Should return original byte count.
	if n != len(input) {
		t.Errorf("Write() returned %d bytes, expected %d", n, len(input))
	}

	output := buf.String()

	// Output should NOT contain the actual secret.
	if strings.Contains(output, secretKey) {
		t.Errorf("Output contains unmasked secret: %s", output)
	}

	// Output should contain the mask replacement.
	if !strings.Contains(output, iolib.MaskReplacement) {
		t.Errorf("Output does not contain mask replacement: %s", output)
	}
}

func TestMaskedWriter_PreservesByteCount(t *testing.T) {
	// Create IO context with masking enabled.
	ioCtx, err := iolib.NewContext()
	if err != nil {
		t.Fatalf("Failed to create IO context: %v", err)
	}

	var buf bytes.Buffer

	writer := &maskedWriter{
		underlying: &buf,
		masker:     ioCtx.Masker(),
	}

	// Multiple writes to test byte counting.
	writes := []string{
		"line 1\n",
		"AKIAIOSFODNN7EXAMPLE\n",
		"line 3\n",
	}

	for _, input := range writes {
		n, err := writer.Write([]byte(input))
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Should return original byte count, not masked length.
		if n != len(input) {
			t.Errorf("Write(%q) returned %d bytes, expected %d", input, n, len(input))
		}
	}
}

// neverEOFReader simulates a terminal stdin: reads block forever until the
// process exits, so io.Copy from such a reader never returns on its own.
type neverEOFReader struct{}

func (neverEOFReader) Read(p []byte) (int, error) {
	select {} // Block forever.
}

func TestExecWithPTY_ReturnsWithBlockedStdin(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	cmd := ptyHelperCommand(t, "print-done")
	opts := &Options{
		Stdin:  neverEOFReader{},
		Stdout: &stdout,
	}

	// Regression: the stdin copier must not block completion. Before the fix,
	// ExecWithPTY joined the stdin copy goroutine, which blocks until the next
	// stdin read after the child exits (i.e., until a keypress).
	finished := make(chan error, 1)
	go func() { finished <- ExecWithPTY(ctx, cmd, opts) }()

	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("ExecWithPTY() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ExecWithPTY() did not return after child exit with blocked stdin")
	}

	if !strings.Contains(stdout.String(), "done") {
		t.Errorf("Expected output to contain 'done', got: %s", stdout.String())
	}
}

func TestExecWithPTY_DisableStdinForward(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	cmd := ptyHelperCommand(t, "stdin-marker")
	opts := &Options{
		Stdin:               strings.NewReader("should never be forwarded\n"),
		Stdout:              &stdout,
		DisableStdinForward: true,
	}

	if err := ExecWithPTY(ctx, cmd, opts); err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "is-a-tty") {
		t.Errorf("Expected child to see a TTY on stdin, got: %s", output)
	}
	if !strings.Contains(output, "NO-INPUT") {
		t.Errorf("Expected host stdin not to be forwarded, got: %s", output)
	}
	if strings.Contains(output, "INPUT-RECEIVED") {
		t.Errorf("Host stdin leaked into the PTY: %s", output)
	}
}

func TestExecWithPTY_ForwardsStdin(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	cmd := ptyHelperCommand(t, "stdin-marker")
	opts := &Options{
		Stdin:  strings.NewReader("forwarded input\n"),
		Stdout: &stdout,
	}

	if err := ExecWithPTY(ctx, cmd, opts); err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "is-a-tty") {
		t.Errorf("Expected child to see a TTY on stdin, got: %s", output)
	}
	if !strings.Contains(output, "INPUT-RECEIVED:forwarded input") {
		t.Errorf("Expected host stdin to be forwarded, got: %s", output)
	}
}

func TestExecWithPTY_ReturnsWhenGrandchildHoldsPTY(t *testing.T) {
	if !IsSupported() {
		t.Skipf("PTY not supported on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	// Reproduces the aws ssm session-manager-plugin teardown hang: the shell
	// exits immediately, but the backgrounded sleep inherits the PTY slave
	// and holds it open, so the output copier never gets EIO. Completion must
	// be bounded by the drain deadline, not the grandchild's lifetime.
	cmd := exec.Command("sh", "-c", "echo session-over; sleep 15 & exit 0")
	opts := &Options{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
	}

	start := time.Now()
	err := ExecWithPTY(ctx, cmd, opts)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("ExecWithPTY() took %v; teardown must be bounded by the drain deadline, not the grandchild's lifetime", elapsed)
	}
	if !strings.Contains(stdout.String(), "session-over") {
		t.Errorf("Expected drained output to contain 'session-over', got: %q", stdout.String())
	}
}
