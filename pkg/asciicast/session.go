package asciicast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	defaultWaitTimeout         = 30 * time.Second
	defaultTeardownQuietPeriod = 500 * time.Millisecond
	defaultTeardownMaxWait     = 2 * time.Second
	sessionReadBufferSize      = 4096
)

var (
	// ErrUnknownSessionAction indicates an unsupported scripted session action type.
	ErrUnknownSessionAction = errUtils.ErrUnknownSessionAction
	// ErrWaitTimeout indicates that a wait action did not observe the expected output before its deadline.
	ErrWaitTimeout = errUtils.ErrWaitTimeout
	// ErrUnsupportedCastKey indicates that a key action requested an unknown key sequence.
	ErrUnsupportedCastKey = errUtils.ErrUnsupportedCastKey
)

// SessionAction describes one scripted action to perform in an interactive cast session.
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

// SessionOptions configures a scripted shell session used to generate cast output.
type SessionOptions struct {
	Shell       string
	Dir         string
	Env         map[string]string
	Width       int
	Height      int
	WriteRate   time.Duration
	KeyInterval time.Duration
	Actions     []SessionAction
}

// RunSession executes scripted session actions against an interactive shell.
func RunSession(ctx context.Context, opts *SessionOptions) error {
	defer perf.Track(nil, "asciicast.RunSession")()

	if opts == nil {
		opts = &SessionOptions{}
	}
	normalizeSessionOptions(opts)
	proc, err := startSessionShell(ctx, opts)
	if err != nil {
		return fmt.Errorf("start cast session shell: %w", err)
	}
	defer func() { _ = proc.close() }()

	state := newSessionState(ctx, proc.output, proc.close)
	defer state.stop()

	for i := range opts.Actions {
		if err := runAction(ctx, proc.input, state, &opts.Actions[i], opts); err != nil {
			proc.kill()
			return errors.Join(err, proc.wait())
		}
	}
	waitForSessionQuiet(ctx, state, defaultTeardownQuietPeriod, defaultTeardownMaxWait)
	state.discardOutput()
	return finishSession(ctx, proc, state.done)
}

type sessionProcess struct {
	input              io.WriteCloser
	output             io.Reader
	closeInputOnFinish bool
	close              func() error
	kill               func()
	wait               func() error
}

type sessionState struct {
	mu      sync.Mutex
	output  bytes.Buffer
	discard bool
	changed chan struct{}
	done    chan error
	cancel  context.CancelFunc
}

func normalizeSessionOptions(opts *SessionOptions) {
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

func sessionEnvironment(env map[string]string) []string {
	if len(env) == 0 {
		return os.Environ()
	}
	merged := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			merged[key] = value
		}
	}
	for key, value := range env {
		merged[key] = value
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}
	return out
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

func newSessionState(ctx context.Context, output io.Reader, closeOutput func() error) *sessionState {
	watchCtx, cancel := context.WithCancel(ctx)
	state := &sessionState{
		changed: make(chan struct{}, 1),
		done:    make(chan error, 1),
		cancel:  cancel,
	}
	go state.readOutput(output)
	go func() {
		<-watchCtx.Done()
		if closeOutput != nil {
			_ = closeOutput()
		}
	}()
	return state
}

func (s *sessionState) readOutput(output io.Reader) {
	buf := make([]byte, sessionReadBufferSize)
	for {
		n, readErr := output.Read(buf)
		if n > 0 {
			s.recordOutputChunk(buf[:n])
		}
		if readErr != nil {
			s.finishRead(readErr)
			return
		}
	}
}

func (s *sessionState) recordOutputChunk(chunk []byte) {
	copied := append([]byte(nil), chunk...)
	s.mu.Lock()
	discard := s.discard
	if !discard {
		_, _ = s.output.Write(copied)
	}
	s.mu.Unlock()
	if discard {
		return
	}
	select {
	case s.changed <- struct{}{}:
	default:
	}
	_, _ = iolib.GetContext().Data().Write(copied)
}

func (s *sessionState) finishRead(err error) {
	if isExpectedSessionReadError(err) {
		s.done <- nil
		return
	}
	s.done <- err
}

func (s *sessionState) discardOutput() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discard = true
}

func (s *sessionState) stop() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func isExpectedSessionReadError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || strings.Contains(err.Error(), "input/output error")
}

func finishSession(ctx context.Context, proc *sessionProcess, done <-chan error) error {
	// Send EOT so interactive shells exit without echoing an artificial
	// "exit" command into the recording.
	_, _ = proc.input.Write([]byte{4})
	if proc.closeInputOnFinish {
		_ = proc.input.Close()
	}
	select {
	case err := <-done:
		waitErr := proc.wait()
		if err == nil {
			return waitErr
		}
		if waitErr != nil {
			return errors.Join(err, waitErr)
		}
		return err
	case <-ctx.Done():
		proc.kill()
		return errors.Join(ctx.Err(), proc.wait())
	case <-time.After(2 * time.Second):
		proc.kill()
		return proc.wait()
	}
}

func runAction(ctx context.Context, input io.Writer, state *sessionState, action *SessionAction, opts *SessionOptions) error {
	switch action.Type {
	case "write":
		return runWriteAction(input, action, opts.WriteRate)
	case "key":
		return runKeyAction(input, action, opts.KeyInterval)
	case "pause":
		return runPauseAction(ctx, action)
	case "wait":
		return waitForOutput(ctx, state, action)
	default:
		return fmt.Errorf("%w: %q", ErrUnknownSessionAction, action.Type)
	}
}

func runWriteAction(input io.Writer, action *SessionAction, fallback time.Duration) error {
	rate := fallback
	if action.Rate != "" {
		parsed, err := time.ParseDuration(action.Rate)
		if err != nil {
			return fmt.Errorf("invalid write rate %q: %w", action.Rate, err)
		}
		rate = parsed
	}
	for _, r := range action.Text {
		if _, err := input.Write([]byte(string(r))); err != nil {
			return err
		}
		if rate > 0 {
			time.Sleep(rate)
		}
	}
	return nil
}

func runKeyAction(input io.Writer, action *SessionAction, fallback time.Duration) error {
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
		if _, err := input.Write([]byte(seq)); err != nil {
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
	}
	if seq, ok := sequences[normalized]; ok {
		return seq, nil
	}
	if len(key) == 1 {
		return key, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedCastKey, key)
}
