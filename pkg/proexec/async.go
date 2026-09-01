package proexec

import (
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
)

// asyncFlushCeiling is the fixed, non-configurable best-effort flush window
// for the async default path (research.md Decision 8, SC-004). Not derived
// from user configuration — a delivery outage must never slow down every
// other command beyond this small, predictable ceiling. CaptureAsync races
// the upload against this ceiling and returns as soon as either finishes.
const asyncFlushCeiling = 2 * time.Second

// logKeyCommand is the structured-log key used for the reported command name
// across CaptureAsync's log lines.
const logKeyCommand = "command"

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
	defer perf.Track(atmosConfig, "proexec.SetAtmosConfig")()

	currentAtmosConfig = atmosConfig
}

// pendingAsyncData is a caller-supplied structured Data payload for the next
// CaptureAsync call, read and cleared by CaptureAsync itself. It exists
// because CaptureAsync is hooked at a call site with only (cmd, err)
// available (mirroring telemetry.CaptureCmd's signature), so a command with
// its own structured Data (e.g. list instances, research.md Decision 23) has
// no other way to attach it — unlike the synchronous path's caller-supplied
// parser closure (WithExecMetadataParser, Decision 18).
//
//nolint:gochecknoglobals // Set immediately before the one CaptureAsync call it targets; read-and-cleared, never leaks across invocations.
var pendingAsyncData any

// SetPendingAsyncData registers a structured Data payload for the very next
// CaptureAsync call. CaptureAsync reads and clears it immediately, so it
// must be called only immediately before the command's own CaptureAsync
// invocation — never left set across invocations.
func SetPendingAsyncData(data any) {
	defer perf.Track(nil, "proexec.SetPendingAsyncData")()

	pendingAsyncData = data
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
		log.Debug("Skipping async exec-metadata upload: command is on the synchronous allowlist.", logKeyCommand, commandPath)
		return
	}

	atmosConfig := currentAtmosConfig
	defer perf.Track(atmosConfig, "proexec.CaptureAsync")()

	if !gateOpen(atmosConfig) {
		return
	}

	client, clientErr := pro.NewAtmosProAPIClientFromEnv(atmosConfig)
	if clientErr != nil {
		log.Debug("Skipping async exec-metadata upload: failed to create Atmos Pro client.", "error", clientErr)
		return
	}

	exitCode := errUtils.GetExitCode(err)

	reportedCommand, args, flags := commandArgsAndFlags(cmd)

	// Read and clear the pending structured-data hand-off (if any) so it
	// applies to this one call only and never leaks into the next
	// invocation's CaptureAsync call (research.md Decision 23).
	data := pendingAsyncData
	pendingAsyncData = nil

	in := &ExecRecordInput{Command: reportedCommand, Args: args, Flags: flags, ExitCode: exitCode, Data: data}
	done := make(chan error, 1)
	go func() {
		done <- uploadExecMetadata(in, client, git.NewDefaultGitRepo())
	}()

	// Race the upload against asyncFlushCeiling: block just long enough to
	// maximize the chance the upload is dispatched before the process exits
	// (FR-009), but never longer than the ceiling. The upload goroutine keeps
	// running and still logs its own outcome if it finishes after the
	// ceiling fires and CaptureAsync has already returned.
	select {
	case uploadErr := <-done:
		logUploadOutcome(reportedCommand, uploadErr)
	case <-time.After(asyncFlushCeiling):
		log.Debug("Exec-metadata upload still in flight after flush ceiling; not blocking further.", logKeyCommand, reportedCommand)
		go func() {
			logUploadOutcome(reportedCommand, <-done)
		}()
	}
}

// logUploadOutcome logs a single exec-metadata upload's terminal outcome,
// shared by both CaptureAsync's in-ceiling and background-after-ceiling paths.
func logUploadOutcome(reportedCommand string, uploadErr error) {
	if uploadErr != nil {
		log.Info("Exec-metadata upload finished.", logKeyCommand, reportedCommand, "success", false, "error", uploadErr)
	} else {
		log.Info("Exec-metadata upload finished.", logKeyCommand, reportedCommand, "success", true)
	}
}

// commandArgsAndFlags derives the FR-003b execution-record shape from a
// Cobra command: Command with the leading "atmos" root segment stripped
// (e.g. "terraform plan", not "atmos terraform plan"), Args holding only
// positional arguments, and Flags holding only the CLI flags actually
// passed (Changed == true), matching the shape internal/exec's sync capture
// path already uses.
func commandArgsAndFlags(cmd *cobra.Command) (command string, args []string, flags []string) {
	if cmd == nil {
		return "", nil, nil
	}

	command = cmd.CommandPath()
	if cmd.Root() != nil {
		command = strings.TrimPrefix(command, cmd.Root().Name()+" ")
	}

	args = cmd.Flags().Args()
	flags = FlagsFromCommand(cmd)

	return command, args, flags
}

// FlagsFromCommand derives the execution record's Flags field (FR-003b) from
// a Cobra command: every flag actually passed (Changed == true), as bare
// tokens exactly as typed on the command line — a bool-typed flag (e.g.
// --upload-status) appears alone, with no synthesized value; a value-bearing
// flag (e.g. -s/--stack) contributes its token and value as two separate
// array entries. This is the single source of truth for both the async
// (commandArgsAndFlags, above) and sync (internal/exec's
// captureExecMetadataSync) delivery paths — they MUST NOT diverge (research.md
// Decision 14).
func FlagsFromCommand(cmd *cobra.Command) []string {
	defer perf.Track(nil, "proexec.FlagsFromCommand")()

	if cmd == nil {
		return nil
	}

	var flags []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		flags = append(flags, flagAsTyped(f)...)
	})
	return flags
}

// flagAsTyped renders one Cobra flag as the bare token(s) it was passed on
// the command line (FR-003b, 2026-08-19 clarification): a bool-typed flag
// (e.g. --upload-status) appears alone, with no synthesized value; any other
// flag contributes its token and value as two separate entries.
func flagAsTyped(f *pflag.Flag) []string {
	if f.Value.Type() == "bool" {
		return []string{"--" + f.Name}
	}
	return []string{"--" + f.Name, f.Value.String()}
}

// uploadExecMetadata builds and sends a single execution record. Any failure
// is returned to the caller to log — it never surfaces to the user or alters
// the calling command's outcome.
func uploadExecMetadata(in *ExecRecordInput, client pro.AtmosProAPIClientInterface, gitRepo git.GitRepoInterface) error {
	metrics := processBaseline.Since()
	req, buildErr := buildRecord(in, &metrics, gitRepo)
	if buildErr != nil {
		return buildErr
	}

	return client.UploadExecMetadata(req)
}
