package asciicast

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	durationpkg "github.com/cloudposse/atmos/pkg/duration"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// This file implements execution of one scripted SessionAction at a time
// (write/key/pause/wait/hide/show/screenshot) plus the key-name-to-sequence
// mapping they rely on. session.go owns the surrounding session lifecycle:
// spawning the shell, tracking output, and tearing down.

func runAction(ctx context.Context, input *syncWriter, state *sessionState, action *SessionAction, opts *SessionOptions) error {
	switch action.Type {
	case "playback":
		return runPlaybackAction(ctx, input, state, action, opts)
	case "write":
		return runTypedWrite(ctx, input, state, action, opts.WriteRate)
	case "key":
		state.noteKeyPress(action.Key)
		return runKeyAction(ctx, input, action, opts.KeyInterval)
	case "pause":
		return runPauseAction(ctx, action)
	case "wait":
		return waitForOutput(ctx, state, action)
	case "hide":
		state.setDiscard(true)
		return nil
	case "show":
		state.setDiscard(false)
		state.mu.Lock()
		if state.scopedWaits {
			state.waitOffset = state.output.Len()
		}
		state.mu.Unlock()
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

// runTypedWrite types a write action's text. A newline inside the text submits
// the line exactly as an Enter key does, so each submitted line is recorded for
// echo tracking at the moment its newline is typed.
func runTypedWrite(ctx context.Context, input *syncWriter, state *sessionState, action *SessionAction, fallback time.Duration) error {
	text := action.Text
	if text == "" {
		return runWriteAction(ctx, input, action, fallback)
	}
	write := func(part string) error {
		segment := *action
		segment.Text = part
		return runWriteAction(ctx, input, &segment, fallback)
	}
	for text != "" {
		end := strings.IndexAny(text, "\r\n")
		if end < 0 {
			state.noteTypedWrite(text)
			return write(text)
		}
		if end > 0 {
			state.noteTypedWrite(text[:end])
			if err := write(text[:end]); err != nil {
				return err
			}
		}
		terminator := text[end : end+1]
		if terminator == "\r" && strings.HasPrefix(text[end+1:], "\n") {
			terminator = "\r\n"
		}
		state.noteSubmittedLine()
		if err := write(terminator); err != nil {
			return err
		}
		text = text[end+len(terminator):]
	}
	return nil
}

func runWriteAction(ctx context.Context, input *syncWriter, action *SessionAction, fallback time.Duration) error {
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
			if err := ctx.Err(); err != nil {
				writeErr = err
				return
			}
			if _, err := w.Write([]byte(string(r))); err != nil {
				writeErr = err
				return
			}
			if rate > 0 {
				if err := sleepSessionAction(ctx, rate); err != nil {
					writeErr = err
					return
				}
			}
		}
	})
	return writeErr
}

func runKeyAction(ctx context.Context, input *syncWriter, action *SessionAction, fallback time.Duration) error {
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
			if err := ctx.Err(); err != nil {
				writeErr = err
				return
			}
			if _, err := w.Write([]byte(seq)); err != nil {
				writeErr = err
				return
			}
			if interval > 0 && i < repeat-1 {
				if err := sleepSessionAction(ctx, interval); err != nil {
					writeErr = err
					return
				}
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
	timeout, re, err := parseWaitAction(action)
	if err != nil {
		return err
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var grace graceTimer
	defer grace.stop()
	for {
		if outputMatches(state, action.Text, re) {
			return nil
		}
		// An open echo span closes when its grace period ends even if no new output arrives.
		graceC := grace.arm(state)
		select {
		case <-state.changed:
		case <-graceC:
		case <-deadline.C:
			return ErrWaitTimeout
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// parseWaitAction resolves a wait action's timeout and optional regex.
func parseWaitAction(action *SessionAction) (time.Duration, *regexp.Regexp, error) {
	timeout := defaultWaitTimeout
	if action.Timeout != "" {
		parsed, err := durationpkg.ParseNonNegative(action.Timeout)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid wait timeout %q: %w", action.Timeout, err)
		}
		timeout = parsed
	}
	if action.Regex == "" {
		return timeout, nil, nil
	}
	re, err := regexp.Compile(action.Regex)
	if err != nil {
		return 0, nil, err
	}
	return timeout, re, nil
}

func outputMatches(state *sessionState, text string, re *regexp.Regexp) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	matches := func(candidate string) bool {
		return (text != "" && strings.Contains(candidate, text)) || (re != nil && re.MatchString(candidate))
	}
	// A wait that matches a line the script typed and submitted searches only
	// after that line's echo, so it matches the command's output rather than
	// the terminal echoing the command.
	raw := state.output.Bytes()
	state.echo.resolve(raw, time.Now())
	from := 0
	if state.scopedWaits {
		from = state.waitOffset
	}
	current := string(raw[state.echo.searchStart(from, len(raw), matches):])
	if state.scopedWaits {
		current = ansi.Strip(current)
	}
	matched := matches(current)
	if matched && state.scopedWaits {
		state.waitOffset = state.output.Len()
	}
	return matched
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

func keySequence(key string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	sequences := map[string]string{
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
	}
	if seq, ok := sequences[normalized]; ok {
		return seq, nil
	}
	if seq, ok := ctrlKeySequence(normalized); ok {
		return seq, nil
	}
	if len(key) == 1 {
		return key, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedCastKey, key)
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

// ValidateKey checks a named terminal key before a recording starts.
func ValidateKey(key string) error {
	defer perf.Track(nil, "asciicast.ValidateKey")()

	_, err := keySequence(key)
	return err
}

// sleepSessionAction makes paced keystrokes interruptible, including during a long interval.
func sleepSessionAction(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
