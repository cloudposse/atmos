package asciicast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

const defaultWaitTimeout = 30 * time.Second

var (
	ErrUnknownSessionAction = errors.New("unknown cast session action type")
	ErrWaitTimeout          = errors.New("timed out waiting for cast output")
	ErrUnsupportedCastKey   = errors.New("unsupported cast key")
)

type SessionAction struct {
	Type     string
	Text     string
	Regex    string
	Key      string
	Duration string
	Timeout  string
	Rate     string
	Interval string
	Repeat   int
}

type SessionOptions struct {
	Shell       string
	Width       int
	Height      int
	WriteRate   time.Duration
	KeyInterval time.Duration
	Actions     []SessionAction
}

func RunSession(ctx context.Context, opts SessionOptions) error {
	defer perf.Track(nil, "asciicast.RunSession")()

	opts = normalizeSessionOptions(opts)
	cmd := exec.CommandContext(ctx, sessionShell(opts.Shell)) //nolint:gosec // The shell is user/config supplied for an explicit interactive cast session.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: safePTYSize(opts.Width), Rows: safePTYSize(opts.Height)})
	if err != nil {
		return fmt.Errorf("start cast session shell: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	state := newSessionState(ctx, ptmx)
	defer state.stop()

	for i := range opts.Actions {
		if err := runAction(ctx, ptmx, state, &opts.Actions[i], opts); err != nil {
			_ = cmd.Process.Kill()
			return err
		}
	}
	return finishSession(ctx, cmd, ptmx, state.done)
}

type sessionState struct {
	mu      sync.Mutex
	output  bytes.Buffer
	changed chan struct{}
	done    chan error
}

func normalizeSessionOptions(opts SessionOptions) SessionOptions {
	if opts.Width <= 0 {
		opts.Width = DefaultWidth
	}
	if opts.Height <= 0 {
		opts.Height = DefaultHeight
	}
	if opts.WriteRate < 0 {
		opts.WriteRate = 0
	}
	if opts.KeyInterval < 0 {
		opts.KeyInterval = 0
	}
	return opts
}

func sessionShell(configured string) string {
	if configured != "" {
		return configured
	}
	if shell, ok := os.LookupEnv("SHELL"); ok && shell != "" {
		return shell
	}
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return "/bin/sh"
}

func safePTYSize(value int) uint16 {
	if value <= 0 {
		return 1
	}
	if value > int(^uint16(0)) {
		return ^uint16(0)
	}
	return uint16(value)
}

func newSessionState(ctx context.Context, ptmx *os.File) *sessionState {
	state := &sessionState{
		changed: make(chan struct{}, 1),
		done:    make(chan error, 1),
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				state.mu.Lock()
				_, _ = state.output.Write(chunk)
				state.mu.Unlock()
				select {
				case state.changed <- struct{}{}:
				default:
				}
				_, _ = iolib.GetContext().Data().Write(chunk)
			}
			if readErr != nil {
				if readErr == io.EOF || strings.Contains(readErr.Error(), "input/output error") {
					state.done <- nil
				} else {
					state.done <- readErr
				}
				return
			}
		}
	}()
	go func() {
		<-ctx.Done()
		_ = ptmx.Close()
	}()
	return state
}

func (s *sessionState) stop() {}

func finishSession(ctx context.Context, cmd *exec.Cmd, ptmx *os.File, done <-chan error) error {
	// Send EOT so interactive shells exit without echoing an artificial
	// "exit" command into the recording.
	_, _ = ptmx.Write([]byte{4})
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		return nil
	}
}

func runAction(ctx context.Context, ptyFile *os.File, state *sessionState, action *SessionAction, opts SessionOptions) error {
	switch action.Type {
	case "write":
		return runWriteAction(ptyFile, action, opts.WriteRate)
	case "key":
		return runKeyAction(ptyFile, action, opts.KeyInterval)
	case "pause":
		return runPauseAction(ctx, action)
	case "wait":
		return waitForOutput(ctx, state, action)
	default:
		return fmt.Errorf("%w: %q", ErrUnknownSessionAction, action.Type)
	}
}

func runWriteAction(ptyFile *os.File, action *SessionAction, fallback time.Duration) error {
	rate := fallback
	if action.Rate != "" {
		parsed, err := time.ParseDuration(action.Rate)
		if err != nil {
			return fmt.Errorf("invalid write rate %q: %w", action.Rate, err)
		}
		rate = parsed
	}
	for _, r := range action.Text {
		if _, err := ptyFile.Write([]byte(string(r))); err != nil {
			return err
		}
		if rate > 0 {
			time.Sleep(rate)
		}
	}
	return nil
}

func runKeyAction(ptyFile *os.File, action *SessionAction, fallback time.Duration) error {
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
	for i := 0; i < repeat; i++ {
		if _, err := ptyFile.Write([]byte(seq)); err != nil {
			return err
		}
		if interval > 0 && i < repeat-1 {
			time.Sleep(interval)
		}
	}
	return nil
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
	}
	if seq, ok := sequences[normalized]; ok {
		return seq, nil
	}
	if len(key) == 1 {
		return key, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedCastKey, key)
}
