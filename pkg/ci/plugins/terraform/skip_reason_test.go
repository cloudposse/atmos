package terraform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/providers/gitea"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// A per-operation status the real Gitea provider cannot send for lack of a token is skipped
// at debug level, never as a warning.
func TestCreatePerOperationStatusesWithoutGiteaTokenIsQuiet(t *testing.T) {
	for _, name := range []string{"ATMOS_CI_GITEA_TOKEN", "GITEA_TOKEN", "GITEA_API_URL", "GITHUB_API_URL", "GITEA_SERVER_URL", "ATMOS_CI_GITEA_ALLOW_INSECURE"} {
		t.Setenv(name, "")
	}
	t.Setenv("ATMOS_CI_GITEA_API_URL", "https://gitea.example.com/api/v1")
	out := captureLogAt(t, log.DebugLevel)

	ctx := &plugin.HookContext{
		Config:   &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Checks: schema.CIChecksConfig{Enabled: boolPtr(true)}}},
		Provider: gitea.NewProvider(),
		CICtx:    &provider.Context{Provider: gitea.ProviderName, RepoOwner: "acme", RepoName: "infra", SHA: "0123456789abcdef"},
		Command:  "plan",
		Info:     &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
	}
	result := &plugin.OutputResult{Data: &plugin.TerraformOutputData{ResourceCounts: plugin.ResourceCounts{Create: 1}}}
	(&Plugin{}).createPerOperationStatuses(ctx, result, "atmos", schema.CIChecksStatusesConfig{Add: boolPtr(true)})

	assert.Contains(t, out.String(), "CI per-operation status creation skipped")
	assert.Contains(t, out.String(), "ATMOS_CI_GITEA_TOKEN or GITEA_TOKEN not set")
	assert.NotContains(t, out.String(), "WARN")
}

// captureWarnings captures log output at warn level and restores the logger after the test.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()

	return captureLogAt(t, log.WarnLevel)
}

// captureLogAt captures log output at the given level and restores the logger after the test.
func captureLogAt(t *testing.T, level log.Level) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	oldLevel := log.GetLevel()
	log.SetLevel(level)
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
