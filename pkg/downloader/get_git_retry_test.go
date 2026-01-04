package downloader

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsRetryableGitError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		// Nil error.
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},

		// Connection errors (should retry).
		{
			name:     "connection reset",
			err:      errors.New("read: connection reset by peer"),
			expected: true,
		},
		{
			name:     "connection refused",
			err:      errors.New("dial tcp: connection refused"),
			expected: true,
		},

		// Timeout errors (should retry).
		{
			name:     "timeout",
			err:      errors.New("operation timeout"),
			expected: true,
		},
		{
			name:     "timed out",
			err:      errors.New("i/o timed out"),
			expected: true,
		},

		// EOF errors (should retry).
		{
			name:     "eof",
			err:      errors.New("unexpected EOF"),
			expected: true,
		},

		// Git-specific transient errors (should retry).
		{
			name:     "temporary failure in name resolution",
			err:      errors.New("temporary failure in name resolution"),
			expected: true,
		},
		{
			name:     "could not read from remote repository",
			err:      errors.New("fatal: Could not read from remote repository"),
			expected: true,
		},
		{
			name:     "remote end hung up",
			err:      errors.New("fatal: the remote end hung up unexpectedly"),
			expected: true,
		},

		// SSL/TLS errors (should retry).
		{
			name:     "ssl error",
			err:      errors.New("SSL_connect: SSL_ERROR_SYSCALL"),
			expected: true,
		},
		{
			name:     "tls handshake error",
			err:      errors.New("tls: handshake failure"),
			expected: true,
		},

		// Rate limiting (should retry).
		{
			name:     "rate limit exceeded",
			err:      errors.New("rate limit exceeded"),
			expected: true,
		},
		{
			name:     "too many requests",
			err:      errors.New("HTTP 429: too many requests"),
			expected: true,
		},

		// HTTP server errors (should retry).
		{
			name:     "service unavailable",
			err:      errors.New("HTTP 503: service unavailable"),
			expected: true,
		},
		{
			name:     "internal server error",
			err:      errors.New("HTTP 500: internal server error"),
			expected: true,
		},
		{
			name:     "bad gateway",
			err:      errors.New("HTTP 502: bad gateway"),
			expected: true,
		},
		{
			name:     "gateway timeout",
			err:      errors.New("HTTP 504: gateway timeout"),
			expected: true,
		},

		// Non-retryable errors (should NOT retry).
		{
			name:     "authentication failed",
			err:      errors.New("Authentication failed for 'https://github.com/org/repo'"),
			expected: false,
		},
		{
			name:     "repository not found",
			err:      errors.New("fatal: repository 'https://github.com/org/repo' not found"),
			expected: false,
		},
		{
			name:     "permission denied",
			err:      errors.New("Permission denied (publickey)"),
			expected: false,
		},
		{
			name:     "reference not found",
			err:      errors.New("fatal: couldn't find remote ref v99.99.99"),
			expected: false,
		},
		{
			name:     "not a git repository",
			err:      errors.New("fatal: not a git repository"),
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("some random error"),
			expected: false,
		},

		// Mixed case (should retry - case insensitive).
		{
			name:     "CONNECTION RESET uppercase",
			err:      errors.New("CONNECTION RESET BY PEER"),
			expected: true,
		},
		{
			name:     "TiMeOuT mixed case",
			err:      errors.New("Operation TiMeOuT occurred"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableGitError(tt.err)
			assert.Equal(t, tt.expected, result, "isRetryableGitError(%v) = %v, want %v", tt.err, result, tt.expected)
		})
	}
}

func TestIsGitHubHTTPURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		// Raw GitHub content URLs (should detect).
		{
			name:     "raw githubusercontent file",
			url:      "https://raw.githubusercontent.com/cloudposse/terraform-aws-vpc/main/README.md",
			expected: true,
		},
		{
			name:     "raw githubusercontent with ref",
			url:      "https://raw.githubusercontent.com/org/repo/v1.0.0/file.yaml",
			expected: true,
		},

		// GitHub archive URLs (should detect).
		{
			name:     "github archive tarball",
			url:      "https://github.com/cloudposse/terraform-aws-vpc/archive/refs/tags/v1.0.0.tar.gz",
			expected: true,
		},
		{
			name:     "github archive zipball",
			url:      "https://github.com/org/repo/archive/main.zip",
			expected: true,
		},

		// GitHub release URLs (should detect).
		{
			name:     "github releases download",
			url:      "https://github.com/hashicorp/terraform/releases/download/v1.5.0/terraform_1.5.0_linux_amd64.zip",
			expected: true,
		},

		// Non-GitHub URLs (should NOT detect).
		{
			name:     "gitlab raw",
			url:      "https://gitlab.com/org/repo/-/raw/main/file.yaml",
			expected: false,
		},
		{
			name:     "bitbucket raw",
			url:      "https://bitbucket.org/org/repo/raw/main/file.yaml",
			expected: false,
		},
		{
			name:     "s3 bucket",
			url:      "s3://my-bucket/path/to/file.tar.gz",
			expected: false,
		},
		{
			name:     "local file",
			url:      "file:///path/to/local/file",
			expected: false,
		},

		// GitHub clone URLs (should NOT detect - these use git protocol).
		{
			name:     "github clone url",
			url:      "https://github.com/org/repo.git",
			expected: false,
		},
		{
			name:     "github clone url without .git",
			url:      "https://github.com/org/repo",
			expected: false,
		},
		{
			name:     "github clone with ref",
			url:      "github.com/org/repo?ref=v1.0.0",
			expected: false,
		},

		// Case insensitive.
		{
			name:     "uppercase raw githubusercontent",
			url:      "HTTPS://RAW.GITHUBUSERCONTENT.COM/org/repo/main/file.yaml",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitHubHTTPURL(tt.url)
			assert.Equal(t, tt.expected, result, "isGitHubHTTPURL(%s) = %v, want %v", tt.url, result, tt.expected)
		})
	}
}

// writeCountingFakeGit creates a fake git that tracks invocation count via a file.
// It fails with a transient error on the first N invocations, then succeeds.
func writeCountingFakeGit(t *testing.T, failCount int, errorMsg string) string {
	t.Helper()

	dir := t.TempDir()
	counterFile := filepath.Join(dir, "counter")

	// Initialize counter to 0.
	err := os.WriteFile(counterFile, []byte("0"), 0o644)
	require.NoError(t, err)

	var fname string
	if runtime.GOOS == "windows" {
		fname = filepath.Join(dir, "git.bat")
		// Windows batch script that reads/increments a counter file.
		script := `@echo off
setlocal enabledelayedexpansion
set /p count=<"` + counterFile + `"
set /a count+=1
echo !count!>"` + counterFile + `"
if !count! leq ` + intToString(failCount) + ` (
    echo ` + errorMsg + ` 1>&2
    exit /b 1
)
exit /b 0
`
		if err := os.WriteFile(fname, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake git: %v", err)
		}
	} else {
		fname = filepath.Join(dir, "git")
		// Shell script that reads/increments a counter file.
		script := `#!/bin/sh
count=$(cat "` + counterFile + `")
count=$((count + 1))
echo "$count" > "` + counterFile + `"
if [ "$count" -le ` + intToString(failCount) + ` ]; then
    echo "` + errorMsg + `" >&2
    exit 1
fi
exit 0
`
		if err := os.WriteFile(fname, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake git: %v", err)
		}
	}

	// Prepend to PATH so our fake is found first.
	oldPath := os.Getenv("PATH")
	newPath := dir
	if oldPath != "" {
		newPath = dir + string(os.PathListSeparator) + oldPath
	}
	t.Setenv("PATH", newPath)

	return counterFile
}

// readCounter reads the invocation count from the counter file.
func readCounter(t *testing.T, counterFile string) int {
	t.Helper()
	data, err := os.ReadFile(counterFile)
	require.NoError(t, err)
	countStr := strings.TrimSpace(string(data))
	count, err := strconv.Atoi(countStr)
	require.NoError(t, err)
	return count
}

func TestGetRunCommandWithRetry_NoRetryConfig(t *testing.T) {
	// When RetryConfig is nil, it should not retry.
	writeCountingFakeGit(t, 1, "connection reset by peer")

	getter := &CustomGitGetter{
		RetryConfig: nil,
	}

	cmd := exec.Command("git", "version")
	ctx := context.Background()
	err := getter.getRunCommandWithRetry(ctx, cmd)

	// Should fail on first attempt without retry.
	assert.Error(t, err)
}

func TestGetRunCommandWithRetry_MaxAttemptsOne(t *testing.T) {
	// When MaxAttempts is 1, it should not retry (same as no retry).
	writeCountingFakeGit(t, 1, "connection reset by peer")

	getter := &CustomGitGetter{
		RetryConfig: &schema.RetryConfig{
			MaxAttempts: 1,
		},
	}

	cmd := exec.Command("git", "version")
	ctx := context.Background()
	err := getter.getRunCommandWithRetry(ctx, cmd)

	// Should fail on first attempt without retry.
	assert.Error(t, err)
}

func TestGetRunCommandWithRetry_SucceedsAfterRetry(t *testing.T) {
	// Simulate a transient error that succeeds on the second attempt.
	counterFile := writeCountingFakeGit(t, 1, "connection reset by peer")

	getter := &CustomGitGetter{
		RetryConfig: &schema.RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			MaxElapsedTime:  30 * time.Second,
			BackoffStrategy: schema.BackoffConstant,
		},
	}

	cmd := exec.Command("git", "version")
	ctx := context.Background()
	err := getter.getRunCommandWithRetry(ctx, cmd)

	// Should succeed after retry.
	assert.NoError(t, err)

	// Verify it was called twice.
	count := readCounter(t, counterFile)
	assert.Equal(t, 2, count, "expected 2 git invocations (1 failure + 1 success)")
}

func TestGetRunCommandWithRetry_SucceedsAfterMultipleRetries(t *testing.T) {
	// Simulate multiple transient errors before success.
	counterFile := writeCountingFakeGit(t, 2, "timeout")

	getter := &CustomGitGetter{
		RetryConfig: &schema.RetryConfig{
			MaxAttempts:     5,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			MaxElapsedTime:  30 * time.Second,
			BackoffStrategy: schema.BackoffConstant,
		},
	}

	cmd := exec.Command("git", "version")
	ctx := context.Background()
	err := getter.getRunCommandWithRetry(ctx, cmd)

	// Should succeed after retries.
	assert.NoError(t, err)

	// Verify it was called 3 times (2 failures + 1 success).
	count := readCounter(t, counterFile)
	assert.Equal(t, 3, count, "expected 3 git invocations (2 failures + 1 success)")
}

func TestGetRunCommandWithRetry_ExhaustsAllAttempts(t *testing.T) {
	// Simulate persistent errors that exhaust all retry attempts.
	counterFile := writeCountingFakeGit(t, 10, "connection refused")

	getter := &CustomGitGetter{
		RetryConfig: &schema.RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			MaxElapsedTime:  30 * time.Second,
			BackoffStrategy: schema.BackoffConstant,
		},
	}

	cmd := exec.Command("git", "version")
	ctx := context.Background()
	err := getter.getRunCommandWithRetry(ctx, cmd)

	// Should fail after exhausting all attempts.
	assert.Error(t, err)

	// Verify it was called MaxAttempts times.
	count := readCounter(t, counterFile)
	assert.Equal(t, 3, count, "expected 3 git invocations (all failed)")
}

func TestGetRunCommandWithRetry_NonRetryableError(t *testing.T) {
	// Simulate a non-retryable error (e.g., authentication failure).
	counterFile := writeCountingFakeGit(t, 10, "authentication failed")

	getter := &CustomGitGetter{
		RetryConfig: &schema.RetryConfig{
			MaxAttempts:     5,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			MaxElapsedTime:  30 * time.Second,
			BackoffStrategy: schema.BackoffConstant,
		},
	}

	cmd := exec.Command("git", "version")
	ctx := context.Background()
	err := getter.getRunCommandWithRetry(ctx, cmd)

	// Should fail immediately without retry.
	assert.Error(t, err)

	// Verify it was called only once (no retry for non-retryable errors).
	count := readCounter(t, counterFile)
	assert.Equal(t, 1, count, "expected 1 git invocation (non-retryable error, no retry)")
}

func TestGetRunCommandWithRetry_ContextCancelled(t *testing.T) {
	// Simulate context cancellation during retry.
	counterFile := writeCountingFakeGit(t, 10, "connection reset by peer")

	getter := &CustomGitGetter{
		RetryConfig: &schema.RetryConfig{
			MaxAttempts:     5,
			InitialDelay:    100 * time.Millisecond, // Longer delay to allow cancellation.
			MaxDelay:        1 * time.Second,
			MaxElapsedTime:  30 * time.Second,
			BackoffStrategy: schema.BackoffConstant,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.Command("git", "version")

	// Start the command in a goroutine and cancel after a short delay.
	done := make(chan error, 1)
	go func() {
		done <- getter.getRunCommandWithRetry(ctx, cmd)
	}()

	// Wait a bit for the first attempt to fail, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done

	// Should fail due to context cancellation.
	assert.Error(t, err)

	// Verify it was called at least once but not all 5 times.
	count := readCounter(t, counterFile)
	assert.GreaterOrEqual(t, count, 1, "expected at least 1 git invocation")
	assert.Less(t, count, 5, "expected fewer than 5 git invocations due to cancellation")
}

func TestGetRunCommandWithRetry_ImmediateSuccess(t *testing.T) {
	// Simulate immediate success with no failures.
	counterFile := writeCountingFakeGit(t, 0, "")

	getter := &CustomGitGetter{
		RetryConfig: &schema.RetryConfig{
			MaxAttempts:     3,
			InitialDelay:    10 * time.Millisecond,
			MaxDelay:        100 * time.Millisecond,
			MaxElapsedTime:  30 * time.Second,
			BackoffStrategy: schema.BackoffConstant,
		},
	}

	cmd := exec.Command("git", "version")
	ctx := context.Background()
	err := getter.getRunCommandWithRetry(ctx, cmd)

	// Should succeed on first attempt.
	assert.NoError(t, err)

	// Verify it was called only once.
	count := readCounter(t, counterFile)
	assert.Equal(t, 1, count, "expected 1 git invocation (immediate success)")
}

func TestCustomGitGetterWithRetryConfig(t *testing.T) {
	// Test that CustomGitGetter properly holds and uses RetryConfig.
	retryConfig := &schema.RetryConfig{
		MaxAttempts:     5,
		InitialDelay:    2 * time.Second,
		MaxDelay:        30 * time.Second,
		BackoffStrategy: schema.BackoffExponential,
		Multiplier:      2.0,
		RandomJitter:    0.1,
	}

	getter := &CustomGitGetter{
		RetryConfig: retryConfig,
	}

	assert.NotNil(t, getter.RetryConfig)
	assert.Equal(t, 5, getter.RetryConfig.MaxAttempts)
	assert.Equal(t, 2*time.Second, getter.RetryConfig.InitialDelay)
	assert.Equal(t, 30*time.Second, getter.RetryConfig.MaxDelay)
	assert.Equal(t, schema.BackoffExponential, getter.RetryConfig.BackoffStrategy)
	assert.Equal(t, 2.0, getter.RetryConfig.Multiplier)
	assert.Equal(t, 0.1, getter.RetryConfig.RandomJitter)
}
