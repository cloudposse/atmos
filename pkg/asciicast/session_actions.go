package asciicast

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	durationpkg "github.com/cloudposse/atmos/pkg/duration"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// This file implements execution of one scripted SessionAction at a time
// (write/key/pause/wait/hide/show/screenshot) plus the key-name-to-sequence
// mapping they rely on. session.go owns the surrounding session lifecycle:
// spawning the shell, tracking output, and tearing down.

const (
	keySettleQuietPeriod = 15 * time.Millisecond
	keySettleMaxWait     = 100 * time.Millisecond
)

func runAction(ctx context.Context, input *syncWriter, state *sessionState, action *SessionAction, opts *SessionOptions) error {
	switch action.Type {
	case "playback":
		return runPlaybackAction(ctx, input, state, action, opts)
	case "write":
		state.markInput()
		return runWriteAction(input, action, opts.WriteRate)
	case "key":
		// Let the echo of previously typed text land before marking, so a key
		// such as Enter does not leave the command's own echo "after" the mark.
		waitForSessionQuiet(ctx, state, keySettleQuietPeriod, keySettleMaxWait)
		state.markInput()
		return runKeyAction(input, action, opts.KeyInterval)
	case "pause":
		return runPauseAction(ctx, action)
	case "wait":
		return waitForOutput(ctx, state, action)
	case "hide":
		state.setDiscard(true)
		return nil
	case "show":
		state.setDiscard(false)
		return nil
	default:
		return runCallbackAction(action)
	}
}

// runCallbackAction dispatches session actions that don't drive the PTY
// directly: a recorder-only marker ("screenshot") or a caller-supplied
// callback ("simulate"). Split out of runAction to keep its cyclomatic
// complexity within limits.
func runCallbackAction(action *SessionAction) error {
	switch action.Type {
	case "screenshot":
		return runScreenshotAction(action)
	case "simulate":
		if action.Fn == nil {
			return ErrSimulateActionMissingCallback
		}
		return action.Fn()
	default:
		return fmt.Errorf("%w: %q", ErrUnknownSessionAction, action.Type)
	}
}

// runScreenshotAction records a named checkpoint marker in the live recording
// at the current point in the session, for later lookup by
// RenderMarkerScreenshots. It writes no bytes to the PTY itself.
func runScreenshotAction(action *SessionAction) error {
	if action.Path == "" {
		return ErrScreenshotActionRequiresPath
	}
	iolib.RecordMarker(action.Path)
	return nil
}

func runWriteAction(input *syncWriter, action *SessionAction, fallback time.Duration) error {
	rate := fallback
	if action.Rate != "" {
		parsed, err := durationpkg.ParseNonNegative(action.Rate)
		if err != nil {
			return fmt.Errorf("invalid write rate %q: %w", action.Rate, err)
		}
		rate = parsed
	}
	// The whole typed string is one atomic unit: holding the lock only
	// per-rune still lets a concurrent terminal-query response land between
	// two characters of the same command, and some shells (observed with
	// real bash, not zsh/sh) re-issue additional queries when a response is
	// delayed, compounding rather than avoiding corruption. See
	// syncWriter.Locked.
	var writeErr error
	input.Locked(func(w io.Writer) {
		for _, r := range action.Text {
			if _, err := w.Write([]byte(string(r))); err != nil {
				writeErr = err
				return
			}
			if rate > 0 {
				time.Sleep(rate)
			}
		}
	})
	return writeErr
}

func runKeyAction(input *syncWriter, action *SessionAction, fallback time.Duration) error {
	repeat := action.Repeat
	if repeat <= 0 {
		repeat = 1
	}
	seq, err := keySequence(action.Key)
	if err != nil {
		return err
	}
	interval, err := keyInterval(action.Interval, fallback)
	if err != nil {
		return err
	}
	// See runWriteAction: the whole repeated sequence must be atomic.
	var writeErr error
	input.Locked(func(w io.Writer) {
		for i := 0; i < repeat; i++ {
			if _, err := w.Write([]byte(seq)); err != nil {
				writeErr = err
				return
			}
			if interval > 0 && i < repeat-1 {
				time.Sleep(interval)
			}
		}
	})
	return writeErr
}

func keyInterval(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := durationpkg.ParseNonNegative(value)
	if err != nil {
		return 0, fmt.Errorf("invalid key interval %q: %w", value, err)
	}
	return parsed, nil
}

func runPauseAction(ctx context.Context, action *SessionAction) error {
	duration, err := durationpkg.ParseNonNegative(action.Duration)
	if err != nil {
		return fmt.Errorf("invalid pause duration %q: %w", action.Duration, err)
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitForOutput(ctx context.Context, state *sessionState, action *SessionAction) error {
	timeout := defaultWaitTimeout
	if action.Timeout != "" {
		parsed, err := durationpkg.ParseNonNegative(action.Timeout)
		if err != nil {
			return fmt.Errorf("invalid wait timeout %q: %w", action.Timeout, err)
		}
		timeout = parsed
	}
	var re *regexp.Regexp
	var err error
	if action.Regex != "" {
		re, err = regexp.Compile(action.Regex)
		if err != nil {
			return err
		}
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if outputMatches(state, action.Text, re) {
			return nil
		}
		select {
		case <-state.changed:
		case <-deadline.C:
			return fmt.Errorf("%w: %s not seen within %s", ErrWaitTimeout, describeWaitTarget(action), timeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// describeWaitTarget renders what a wait action was looking for, for the
// timeout error: the /regex/ when one was given, otherwise the quoted text.
func describeWaitTarget(action *SessionAction) string {
	if action.Regex != "" {
		return "/" + action.Regex + "/"
	}
	return strconv.Quote(action.Text)
}

// outputMatches reports whether text or re matches output received since the
// most recent input action began; see sessionState.inputOffset.
func outputMatches(state *sessionState, text string, re *regexp.Regexp) bool {
	current := state.outputSinceInput()
	return (text != "" && strings.Contains(current, text)) || (re != nil && re.MatchString(current))
}

func waitForSessionQuiet(ctx context.Context, state *sessionState, quiet, maxWait time.Duration) {
	if state == nil || quiet <= 0 || maxWait <= 0 {
		return
	}
	quietTimer := time.NewTimer(quiet)
	defer quietTimer.Stop()
	maxTimer := time.NewTimer(maxWait)
	defer maxTimer.Stop()

	for {
		select {
		case <-state.changed:
			resetTimer(quietTimer, quiet)
		case <-quietTimer.C:
			return
		case <-maxTimer.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

var keySequences = map[string]string{
	"enter":     "\r",
	"return":    "\r",
	"tab":       "\t",
	"esc":       "\x1b",
	"escape":    "\x1b",
	"backspace": "\x7f",
	"space":     " ",
	"up":        "\x1b[A",
	"down":      "\x1b[B",
	"right":     "\x1b[C",
	"left":      "\x1b[D",
	"pageup":    "\x1b[5~",
	"pagedown":  "\x1b[6~",
	"delete":    "\x1b[3~",
	"insert":    "\x1b[2~",
	"home":      "\x1b[H",
	"end":       "\x1b[F",
	"shift+tab": "\x1b[Z",
}

func keySequence(key string) (string, error) {
	trimmed := strings.TrimSpace(key)
	normalized := strings.ToLower(trimmed)
	if seq, ok := keySequences[normalized]; ok {
		return seq, nil
	}
	if seq, ok := ctrlKeySequence(normalized); ok {
		return seq, nil
	}
	if seq, ok := modifiedKeySequence(trimmed, normalized); ok {
		return seq, nil
	}
	if len(key) == 1 {
		return key, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedCastKey, key)
}

// modifiedKeySequence maps three modifier forms. "alt+<character>" becomes ESC
// then the character, "shift+<letter>" becomes the uppercase letter, and
// "ctrl+alt+<letter>" becomes ESC then the control code. The trimmed argument
// keeps the caller's casing for the Alt character.
func modifiedKeySequence(trimmed, normalized string) (string, bool) {
	const (
		altPrefix     = "alt+"
		shiftPrefix   = "shift+"
		ctrlAltPrefix = "ctrl+alt+"
	)
	if rest, ok := strings.CutPrefix(normalized, ctrlAltPrefix); ok {
		ctrl, ok := ctrlKeySequence("ctrl+" + rest)
		return "\x1b" + ctrl, ok
	}
	if rest, ok := strings.CutPrefix(normalized, shiftPrefix); ok {
		if len(rest) == 1 && rest[0] >= 'a' && rest[0] <= 'z' {
			return strings.ToUpper(rest), true
		}
		return "", false
	}
	if strings.HasPrefix(normalized, altPrefix) {
		char := trimmed[len(altPrefix):]
		if utf8.RuneCountInString(char) == 1 {
			return "\x1b" + char, true
		}
	}
	return "", false
}

// ctrlKeySequence maps "ctrl+<letter>" (VHS's modifier-key spelling) to the
// corresponding ASCII control code, e.g. "ctrl+c" -> 0x03.
func ctrlKeySequence(normalized string) (string, bool) {
	const prefix = "ctrl+"
	if !strings.HasPrefix(normalized, prefix) || len(normalized) != len(prefix)+1 {
		return "", false
	}
	letter := normalized[len(prefix)]
	if letter < 'a' || letter > 'z' {
		return "", false
	}
	return string(rune(letter - 'a' + 1)), true
}
