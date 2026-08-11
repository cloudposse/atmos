package proexec

import (
	"sync"
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
func CaptureAsync(cmd *cobra.Command, err error) {
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

	commandPath := ""
	if cmd != nil {
		commandPath = cmd.CommandPath()
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		uploadExecMetadata(atmosConfig, commandPath, exitCode, nil, client, git.NewDefaultGitRepo())
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(asyncFlushCeiling):
		// Best-effort ceiling reached; the goroutine is abandoned and the
		// process is free to exit — Go permits this.
		log.Debug("Async exec-metadata upload did not finish within the flush ceiling; abandoning.")
	}
}

// uploadExecMetadata builds and sends a single execution record. Any failure
// is logged at debug level only (FR-009a) and never surfaces to the user or
// alters the calling command's outcome.
func uploadExecMetadata(
	atmosConfig *schema.AtmosConfiguration,
	commandPath string,
	exitCode int,
	data any,
	client pro.AtmosProAPIClientInterface,
	gitRepo git.GitRepoInterface,
) {
	req, buildErr := buildRecord(atmosConfig, commandPath, exitCode, processBaseline.Since(), data, gitRepo)
	if buildErr != nil {
		log.Debug("Skipping exec-metadata upload: failed to build execution record.", "error", buildErr)
		return
	}

	if uploadErr := client.UploadExecMetadata(req); uploadErr != nil {
		log.Debug("Exec-metadata upload failed.", "error", uploadErr)
	}
}
