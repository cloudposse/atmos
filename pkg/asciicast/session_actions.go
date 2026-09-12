package asciicast

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// This file implements execution of one scripted SessionAction at a time
// (write/key/pause/wait/hide/show/screenshot) plus the key-name-to-sequence
// mapping they rely on. session.go owns the surrounding session lifecycle:
// spawning the shell, tracking output, and tearing down.

func runAction(ctx context.Context, input *syncWriter, state *sessionState, action *SessionAction, opts *SessionOptions) error {
	switch action.Type {
	case "write":
		return runWriteAction(input, action, opts.WriteRate)
	case "key":
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
		parsed, err := time.ParseDuration(action.Rate)
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
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid key interval %q: %w", value, err)
	}
	return parsed, nil
}

func runPauseAction(ctx context.Context, action *SessionAction) error {
	duration, err := time.ParseDuration(action.Duration)
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
		parsed, err := time.ParseDuration(action.Timeout)
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
			return ErrWaitTimeout
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func outputMatches(state *sessionState, text string, re *regexp.Regexp) bool {
	state.mu.Lock()
	current := state.output.String()
	state.mu.Unlock()
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
