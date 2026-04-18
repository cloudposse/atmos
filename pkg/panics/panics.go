// Package panics provides a process-wide panic handler that converts
// uncaught panics into a friendly, actionable crash message while
// preserving the full stack trace for bug reports.
//
// Typical usage at a goroutine entry point (most importantly, main.run):
//
//	func run() int {
//	    defer panics.Recover(&exitCode)
//	    ...
//	}
//
// The handler:
//   - prints a short headline + body via pkg/ui (never raw fmt.Fprintf),
//   - gates the full stack trace behind ATMOS_LOGS_LEVEL=Debug/Trace
//     (always writes the stack to a temp crash-report file so the user
//     can attach it to a GitHub issue),
//   - routes the panic through errUtils.CaptureError so Sentry receives it,
//   - returns exit code 1 (mirroring the existing error-exit convention).
//
// Note: recover() only catches panics on the calling goroutine. Spawned
// goroutines (signal handler, background telemetry, etc.) still need
// their own deferred Recover() at their entry point.
package panics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	cockroachErrors "github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/version"
)

const (
	// IssueTrackerURL is where users are directed to report crashes.
	// Exported so other packages can reference the same canonical URL.
	IssueTrackerURL = "https://github.com/cloudposse/atmos/issues"

	// PanicExitCode is the exit code returned after a handled panic.
	// Kept at 1 to match the existing error-exit convention so CI
	// behavior is unchanged. Callers wanting a distinct code for panics
	// can wrap HandlePanic directly.
	PanicExitCode = 1

	// CrashReportFileMode locks the crash report down to the invoking
	// user because it can contain argv / path values that leak
	// sensitive data.
	CrashReportFileMode os.FileMode = 0o600
)

// Recover is the standard deferred panic handler. Call it as the very
// first deferred statement in any goroutine whose crash should be
// intercepted. If the caller wants the resulting exit code (typically
// main.run), pass a non-nil pointer to be populated on recovery.
//
// Usage:
//
//	func run() int {
//	    exitCode := 0
//	    defer panics.Recover(&exitCode)
//	    ...
//	    return exitCode
//	}
//
// If no panic occurred, *exitCode is left untouched and Recover
// returns silently.
func Recover(exitCode *int) {
	defer perf.Track(nil, "panics.Recover")()

	r := recover()
	if r == nil {
		return
	}
	opts := defaultOptions()
	code := HandlePanic(r, debug.Stack(), &opts)
	if exitCode != nil {
		*exitCode = code
	}
}

// HandlePanic is the core panic-to-friendly-message pipeline, exposed
// for testing. Production code should call Recover(). It does NOT call
// os.Exit — the caller owns process termination.
//
// Returns the exit code the caller should propagate.
func HandlePanic(panicValue any, stack []byte, opts *Options) int {
	defer perf.Track(nil, "panics.HandlePanic")()

	if opts == nil {
		o := Options{}
		opts = &o
	}
	opts.applyDefaults()

	summary := summarize(panicValue)

	// Write the crash report first. If the OS can't give us a temp
	// file, we still want to show the user the friendly message, so
	// we treat the write failure as non-fatal.
	reportPath, writeErr := writeCrashReport(opts, panicValue, stack)
	if writeErr != nil {
		log.Warn("failed to write crash report", "error", writeErr)
	}

	// Send to Sentry (safe no-op if not initialized). Wrap with a
	// stack so cockroachdb/errors/Sentry have full context.
	wrapped := cockroachErrors.WithStack(cockroachErrors.Newf("panic: %s", summary))
	errUtils.CaptureError(wrapped)

	// Structured log at Error level so observability pipelines capture
	// the crash even when the user pipes stderr elsewhere.
	log.Error("atmos panic recovered",
		"summary", summary,
		"version", version.Version,
		"report", reportPath,
	)

	// User-facing output — prefer pkg/ui. If ui isn't initialized
	// (panic happened before InitFormatter), ui.* is a no-op per
	// its own contract; we print a plain fallback in that case.
	headline := "Atmos crashed unexpectedly"
	body := buildFriendlyMessage(summary, reportPath, opts)

	if opts.useUI {
		ui.Error(headline)
		ui.MarkdownMessage(body)
	} else {
		fallbackWrite(opts.stderr, headline+"\n\n"+body+"\n")
	}

	// If the user asked for it, dump the full stack inline after the
	// friendly message — debugging convenience for contributors.
	if opts.showStackInline {
		if opts.useUI {
			ui.Write("\nStack trace:\n" + string(stack))
		} else {
			fallbackWrite(opts.stderr, "\nStack trace:\n"+string(stack))
		}
	}

	return opts.exitCode
}

// summarize renders a single-line summary of the panic value.
func summarize(panicValue any) string {
	switch v := panicValue.(type) {
	case runtime.Error:
		return v.Error()
	case error:
		return v.Error()
	case string:
		return v
	default:
		return fmt.Sprintf("%v", panicValue)
	}
}

// buildFriendlyMessage renders the markdown body shown to users after
// the red "Atmos crashed" headline.
func buildFriendlyMessage(summary, reportPath string, opts *Options) string {
	var b strings.Builder
	b.WriteString("**Summary:** ")
	b.WriteString(summary)
	b.WriteString("\n\n")

	b.WriteString("This is a bug in Atmos. Please report it at:\n\n")
	b.WriteString("  ")
	b.WriteString(IssueTrackerURL)
	b.WriteString("\n\n")

	b.WriteString("Please include the following when you file the issue:\n\n")
	fmt.Fprintf(&b, "- **Version:** `%s`\n", version.Version)
	fmt.Fprintf(&b, "- **OS / Arch:** `%s/%s`\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- **Built with:** `%s`\n", runtime.Version())
	if len(opts.args) > 0 {
		fmt.Fprintf(&b, "- **Command:** `%s`\n", strings.Join(opts.args, " "))
	}
	if reportPath != "" {
		fmt.Fprintf(&b, "- **Crash report:** `%s`\n", reportPath)
	}
	b.WriteString("\n")

	if !opts.showStackInline {
		b.WriteString("Re-run with `ATMOS_LOGS_LEVEL=Debug` (or `--logs-level=Debug`) to see the full stack trace inline.\n")
	}
	return b.String()
}

// writeCrashReport writes the panic, stack, and environment to a temp
// file. Returns the path (empty if the write failed).
func writeCrashReport(opts *Options, panicValue any, stack []byte) (string, error) {
	if opts.crashDir == "" {
		opts.crashDir = os.TempDir()
	}

	timestamp := opts.now().UTC().Format("20060102-150405")
	name := fmt.Sprintf("atmos-crash-%s-%d.txt", timestamp, os.Getpid())
	path := filepath.Join(opts.crashDir, name)

	var b strings.Builder
	b.WriteString("Atmos crashed unexpectedly.\n\n")
	fmt.Fprintf(&b, "Version:    %s\n", version.Version)
	fmt.Fprintf(&b, "Built with: %s\n", runtime.Version())
	fmt.Fprintf(&b, "OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "Time:       %s\n", opts.now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "PID:        %d\n", os.Getpid())
	if len(opts.args) > 0 {
		fmt.Fprintf(&b, "Command:    %s\n", strings.Join(opts.args, " "))
	}
	fmt.Fprintf(&b, "\nPanic: %s\n\n", summarize(panicValue))
	b.WriteString("Stack:\n")
	b.Write(stack)

	if err := os.WriteFile(path, []byte(b.String()), CrashReportFileMode); err != nil {
		return "", err
	}
	return path, nil
}

// fallbackWrite is the last-resort stderr writer used only when
// pkg/ui is not initialized. Errors are ignored — we've already
// panicked once.
func fallbackWrite(w writer, s string) {
	if w == nil {
		return
	}
	_, _ = w.Write([]byte(s))
}
