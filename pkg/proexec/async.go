package proexec

import (
	"time"

	"github.com/spf13/cobra"

	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
)

// asyncFlushCeiling is the fixed, non-configurable best-effort flush window
// for the async default path (research.md Decision 8, SC-004). Not derived
// from user configuration — a delivery outage must never slow down every
// other command beyond this small, predictable ceiling.
//
// TEMPORARILY UNUSED: CaptureAsync currently blocks synchronously on every
// upload (see CaptureAsync below) so its response can be logged reliably
// while the exec-metadata upload path is being validated. Restore the
// select/timeout race once that verification is done.
const asyncFlushCeiling = 2 * time.Second

// processBaseline is captured once, as early as possible in the process's
// lifetime (at package load, before any command runs), so both the async
// default path and any synchronous command's own CaptureSync call diff
// against the same baseline (data-model.md's "Correlation ID"/metrics rules,
// research.md Decision 3).
//
//nolint:gochecknoglobals // Intentional: one baseline per process lifetime.
var processBaseline = process.Baseline()

// currentAtmosConfig is set once by cmd/root.go (mirroring the SetAtmosConfig
// pattern already used by other cmd/* packages) so CaptureAsync — which is
// hooked at a call site with only (cmd, err) available, matching
// telemetry.CaptureCmd's signature — can still reach Settings.Pro without
// threading atmosConfig through every command's call stack.
//
//nolint:gochecknoglobals // Set once at startup; mirrors cmd/root.go's own atmosConfig pattern.
var currentAtmosConfig *schema.AtmosConfiguration

// SetAtmosConfig registers the loaded Atmos configuration for use by
// CaptureAsync. Must be called once during startup (cmd/root.go's Execute),
// before any command runs.
func SetAtmosConfig(atmosConfig *schema.AtmosConfiguration) {
	currentAtmosConfig = atmosConfig
}

// CaptureAsync fires a best-effort, non-blocking execution-record upload for
// the just-completed command. It re-checks the CI+Pro gate itself (callers
// don't need to duplicate that check), never alters err or the caller's exit
// path, and blocks the caller for at most asyncFlushCeiling to maximize the
// chance the upload is dispatched before the process exits (FR-009).
//
// Commands on the synchronous allowlist (IsSyncCommand) are skipped here:
// their own execution path already calls CaptureSync directly, and the two
// delivery paths are mutually exclusive per invocation (FR-007) — a command
// must never produce two execution records for the same run.
func CaptureAsync(cmd *cobra.Command, err error) {
	commandPath := ""
	if cmd != nil {
		commandPath = cmd.CommandPath()
	}

	if IsSyncCommand(commandPath) {
		log.Debug("Skipping async exec-metadata upload: command is on the synchronous allowlist.", "command", commandPath)
		return
	}

	atmosConfig := currentAtmosConfig
	if !gateOpen(atmosConfig) {
		return
	}

	client, clientErr := pro.NewAtmosProAPIClientFromEnv(atmosConfig)
	if clientErr != nil {
		log.Debug("Skipping async exec-metadata upload: failed to create Atmos Pro client.", "error", clientErr)
		return
	}

	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	// TEMPORARY: block on the upload (instead of racing asyncFlushCeiling)
	// so the command's process doesn't exit before the request completes,
	// and so its outcome is always logged.
	uploadErr := uploadExecMetadata(commandPath, exitCode, nil, nil, client, git.NewDefaultGitRepo())
	if uploadErr != nil {
		log.Info("Exec-metadata upload finished.", "command", commandPath, "success", false, "error", uploadErr)
	} else {
		log.Info("Exec-metadata upload finished.", "command", commandPath, "success", true)
	}
}

// uploadExecMetadata builds and sends a single execution record. Any failure
// is returned to the caller to log — it never surfaces to the user or alters
// the calling command's outcome.
func uploadExecMetadata(
	commandPath string,
	exitCode int,
	data any,
	dataItems []any,
	client pro.AtmosProAPIClientInterface,
	gitRepo git.GitRepoInterface,
) error {
	req, buildErr := buildRecord(commandPath, exitCode, processBaseline.Since(), data, dataItems, gitRepo)
	if buildErr != nil {
		return buildErr
	}

	return client.UploadExecMetadata(req)
}
