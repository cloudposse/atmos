package downloader

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	charm "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRetryObserverCommand is a portable subprocess fixture. It also checks the
// working directory and environment that recreated retry commands must preserve.
func TestRetryObserverCommand(t *testing.T) {
	arguments := os.Args
	if len(arguments) < 5 || arguments[len(arguments)-4] != "--retry-observer-helper" {
		return
	}
	counterPath := arguments[len(arguments)-3]
	failures, err := strconv.Atoi(arguments[len(arguments)-2])
	require.NoError(t, err)
	expectedDirectory := arguments[len(arguments)-1]
	directory, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, expectedDirectory, directory)
	require.Equal(t, "inherited", os.Getenv("ATMOS_RETRY_OBSERVER_TEST"))
	count := 0
	if data, err := os.ReadFile(counterPath); err == nil {
		count, err = strconv.Atoi(string(data))
		require.NoError(t, err)
	} else {
		require.True(t, os.IsNotExist(err))
	}
	count++
	require.NoError(t, os.WriteFile(counterPath, []byte(strconv.Itoa(count)), 0o600))
	if count <= failures {
		message := os.Getenv("ATMOS_RETRY_OBSERVER_ERROR")
		if message == "" {
			message = "connection reset by peer"
		}
		fmt.Fprintln(os.Stderr, message)
		os.Exit(1)
	}
}

func retryObserverCommand(t *testing.T, ctx context.Context, directory string, failures int) *exec.Cmd {
	t.Helper()
	directory, err := filepath.EvalSymlinks(directory)
	require.NoError(t, err)
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestRetryObserverCommand$", "--", "--retry-observer-helper", filepath.Join(directory, "counter"), strconv.Itoa(failures), directory)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "ATMOS_RETRY_OBSERVER_TEST=inherited")
	return cmd
}

func TestRetryObserverReportsOnlyActualRetries(t *testing.T) {
	t.Run("subprocess inherits directory and environment", func(t *testing.T) {
		cmd := retryObserverCommand(t, context.Background(), t.TempDir(), 0)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
	})
	for _, tc := range []struct {
		name     string
		failures int
		attempts []int
	}{
		{name: "first attempt success", failures: 0},
		{name: "two retries then success", failures: 2, attempts: []int{2, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts []int
			var logOutput bytes.Buffer
			oldLogger := log.Default()
			log.SetDefault(log.NewAtmosLogger(charm.New(&logOutput)))
			t.Cleanup(func() { log.SetDefault(oldLogger) })
			config := &schema.RetryConfig{MaxAttempts: intPtr(3), InitialDelay: durationPtr(time.Millisecond), MaxDelay: durationPtr(time.Millisecond), MaxElapsedTime: durationPtr(30 * time.Second), BackoffStrategy: schema.BackoffConstant}
			fd := NewGoGetterDownloader(nil, WithHTTPClient(&http.Client{}), WithRetryConfig(config), WithRetryObserver(func(attempt int) { attempts = append(attempts, attempt) })).(*fileDownloader)
			client, err := fd.clientFactory.NewClient(context.Background(), "local-source", "unused", ClientModeFile)
			require.NoError(t, err)
			gitGetter := client.(*goGetterClient).client.Getters["git"].(*CustomGitGetter)
			directory := t.TempDir()
			err = gitGetter.getRunCommandWithRetry(context.Background(), retryObserverCommand(t, context.Background(), directory, tc.failures))
			require.NoError(t, err)
			assert.Equal(t, tc.attempts, attempts)
			assert.Empty(t, logOutput.String(), "observer owns retry progress; worker must not write warning lines")
			count, err := os.ReadFile(filepath.Join(directory, "counter"))
			require.NoError(t, err)
			assert.Equal(t, strconv.Itoa(tc.failures+1), string(count))
		})
	}
}

func TestRetryObserverReturnsBrokerDiagnosticWithoutLogging(t *testing.T) {
	var output bytes.Buffer
	previous := log.Default()
	log.SetDefault(log.NewAtmosLogger(charm.New(&output)))
	t.Cleanup(func() { log.SetDefault(previous) })
	var attempts []int
	getter := CustomGitGetter{RetryAuthErrors: true, OnRetry: func(attempt int) { attempts = append(attempts, attempt) }, RetryConfig: &schema.RetryConfig{
		MaxAttempts: intPtr(2), InitialDelay: durationPtr(time.Millisecond), MaxDelay: durationPtr(time.Millisecond), BackoffStrategy: schema.BackoffConstant,
	}}
	cmd := retryObserverCommand(t, context.Background(), t.TempDir(), 3)
	cmd.Env = append(cmd.Env, "ATMOS_RETRY_OBSERVER_ERROR=authentication failed")
	err := getter.getRunCommandWithRetry(context.Background(), cmd)
	require.ErrorIs(t, err, errUtils.ErrGitCommandExited)
	assert.Contains(t, err.Error(), "verify the STS trust policy")
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Equal(t, []int{2}, attempts)
	assert.Empty(t, output.String())
}
