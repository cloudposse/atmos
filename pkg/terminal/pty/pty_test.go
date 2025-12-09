package pty

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

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

	cmd := exec.Command("echo", "hello world")
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

	// Echo a known AWS access key pattern.
	secretKey := "AKIAIOSFODNN7EXAMPLE"
	cmd := exec.Command("echo", secretKey)

	opts := &Options{
		Stdin:         strings.NewReader(""), // Provide empty stdin for CI environments.
		Stdout:        &stdout,
		Masker:        ioCtx.Masker(),
		EnableMasking: true,
	}

	err = ExecWithPTY(ctx, cmd, opts)
	if err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}

	// Allow time for PTY output buffers to fully flush in CI environments.
	time.Sleep(50 * time.Millisecond)

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
	cmd := exec.Command("echo", secretKey)

	opts := &Options{
		Stdin:         strings.NewReader(""), // Provide empty stdin for CI environments.
		Stdout:        &stdout,
		Masker:        ioCtx.Masker(),
		EnableMasking: false, // Explicitly disabled.
	}

	err = ExecWithPTY(ctx, cmd, opts)
	if err != nil {
		t.Fatalf("ExecWithPTY() error = %v", err)
	}

	// Allow time for PTY output buffers to fully flush in CI environments.
	time.Sleep(50 * time.Millisecond)

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

	cmd := exec.Command("echo", "test")

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
