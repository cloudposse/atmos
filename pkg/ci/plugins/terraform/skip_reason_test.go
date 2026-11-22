package terraform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// captureWarnings captures log output at warn level and restores the logger after the test.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	oldLevel := log.GetLevel()
	log.SetLevel(log.WarnLevel)
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(oldLevel)
	})
	return &buf
}

var errUnexpected = errors.New("boom")

// Configuration gaps are skipped quietly on every provider; real failures stay warnings.
func TestSkippedAPIWritesLogConfigurationGapsQuietly(t *testing.T) {
	wrap := func(sentinel error) error {
		return fmt.Errorf("%w: %w", errUtils.ErrCICheckRunCreateFailed, fmt.Errorf("%w: detail", sentinel))
	}
	loggers := map[string]func(error){
		"check run":           func(err error) { logCheckRunError("CI check run creation skipped", err) },
		"comment":             func(err error) { logCommentError("CI comment skipped", err) },
		"per-operation check": func(err error) { logOperationStatusError("atmos/plan/dev/vpc/add", err) },
	}
	quiet := map[string]error{
		"github token missing":  wrap(errUtils.ErrGitHubTokenNotFound),
		"gitea api unavailable": wrap(errUtils.ErrGiteaAPIUnavailable),
		"gitea token missing":   wrap(errUtils.ErrGiteaTokenNotFound),
	}
	for logger, logFn := range loggers {
		for name, err := range quiet {
			t.Run(logger+"/"+name, func(t *testing.T) {
				buf := captureWarnings(t)
				logFn(err)
				assert.Empty(t, buf.String())
			})
		}
		t.Run(logger+"/unexpected error warns", func(t *testing.T) {
			buf := captureWarnings(t)
			logFn(wrap(errUnexpected))
			assert.Contains(t, buf.String(), "boom")
		})
	}
}
