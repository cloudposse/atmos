// Package interactive decides whether Atmos authentication flows may show an
// interactive prompt (identity selector, MFA token, credential input).
//
// The prompts are rendered with huh, which reads keystrokes from stdin and draws
// the form on stderr. A prompt is therefore only usable when BOTH streams are
// terminals. When stderr is captured (for example by the AWS CLI running a
// credential_process helper) the form is invisible and the process would hang
// waiting for input nobody can see, so the check must fail closed.
package interactive

import (
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// viperKeyInteractive is the viper key bound to --interactive / ATMOS_INTERACTIVE.
const viperKeyInteractive = "interactive"

// Environment bundles the inputs of the interactivity decision so tests can
// inject them without a real terminal.
type Environment struct {
	// TTY reports which standard streams are terminals. It already honors
	// --force-tty / ATMOS_FORCE_TTY when it is the default detector.
	TTY term.TTYDetector
	// Enabled reports whether interactive mode is enabled (--interactive / ATMOS_INTERACTIVE).
	Enabled func() bool
	// CI reports whether the process runs in a CI environment.
	CI func() bool
}

// Default returns the Environment backed by the real terminal, viper, and CI detection.
func Default() Environment {
	return Environment{
		TTY:     &term.DefaultTTYDetector{},
		Enabled: func() bool { return viper.GetBool(viperKeyInteractive) },
		CI:      telemetry.IsCI,
	}
}

// Available reports whether interactive prompts can be shown. It requires:
//  1. Interactive mode is enabled (--interactive flag or ATMOS_INTERACTIVE, default true).
//  2. Stdin is a terminal (the prompt reads keystrokes from it).
//  3. Stderr is a terminal (the prompt is drawn on it).
//  4. The process does not run in CI.
//
// Prompts degrade to actionable errors in pipelines, scripts, CI, and when a
// parent process captures stderr.
func (e Environment) Available() bool {
	defer perf.Track(nil, "interactive.Environment.Available")()

	if e.Enabled != nil && !e.Enabled() {
		return false
	}
	if e.CI != nil && e.CI() {
		return false
	}
	return e.TTY != nil && e.TTY.IsTTYForStdin() && e.TTY.IsTTYForStderr()
}

// Available reports whether interactive prompts can be shown in the current process.
// It is the single predicate every authentication prompt gate must use.
func Available() bool {
	return Default().Available()
}
