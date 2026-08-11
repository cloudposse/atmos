package proexec

import (
	"time"

	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// defaultSyncTimeoutSeconds is the default (and floor) wait for a
// synchronous command's execution-record upload (FR-008a).
const defaultSyncTimeoutSeconds = 10

// CaptureSync uploads an execution record for a command classified as
// synchronous (terraform plan/apply, describe affected), blocking until the
// upload completes or the configured timeout elapses. It re-checks the
// CI+Pro gate itself. Failure/timeout is warn-and-continue for all three
// initial synchronous commands (data-model.md's Delivery Classification
// table): a warning is logged and CaptureSync returns nil so a delivery
// outage never turns a successful command into a failed CI run.
func CaptureSync(atmosConfig *schema.AtmosConfiguration, cmdName string, exitCode int, data any) error {
	if !gateOpen(atmosConfig) {
		return nil
	}

	client, clientErr := pro.NewAtmosProAPIClientFromEnv(atmosConfig)
	if clientErr != nil {
		log.Debug("Skipping sync exec-metadata upload: failed to create Atmos Pro client.", "error", clientErr)
		return nil
	}

	timeout := syncTimeout(atmosConfig)

	resultCh := make(chan error, 1)
	go func() {
		req, buildErr := buildRecord(atmosConfig, cmdName, exitCode, processBaseline.Since(), data, git.NewDefaultGitRepo())
		if buildErr != nil {
			resultCh <- buildErr
			return
		}
		resultCh <- client.UploadExecMetadata(req)
	}()

	select {
	case err := <-resultCh:
		if err != nil {
			ui.Warningf("Failed to report execution metadata to Atmos Pro for %q: %v", cmdName, err)
			log.Warn("Sync exec-metadata upload failed; continuing (warn-and-continue).", "command", cmdName, "error", err)
		}
		return nil
	case <-time.After(timeout):
		ui.Warningf("Timed out waiting to report execution metadata to Atmos Pro for %q after %s.", cmdName, timeout)
		log.Warn("Sync exec-metadata upload timed out; continuing (warn-and-continue).", "command", cmdName, "timeout", timeout)
		return nil
	}
}

// syncTimeout returns the configured synchronous-upload wait, clamped up to
// (never below) the 10-second default — the setting can only lengthen the
// wait, never shorten or disable it (research.md Decision 7).
func syncTimeout(atmosConfig *schema.AtmosConfiguration) time.Duration {
	configured := 0
	if atmosConfig != nil {
		configured = atmosConfig.Settings.Pro.Exec.SyncTimeoutSeconds
	}
	if configured < defaultSyncTimeoutSeconds {
		configured = defaultSyncTimeoutSeconds
	}
	return time.Duration(configured) * time.Second
}
