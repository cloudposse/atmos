package asciicast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	defaultWaitTimeout         = 30 * time.Second
	defaultTeardownQuietPeriod = 500 * time.Millisecond
	defaultTeardownMaxWait     = 2 * time.Second
	defaultProcessExitMaxWait  = 2 * time.Second
	sessionReadBufferSize      = 4096
)

var (
	// ErrUnknownSessionAction indicates an unsupported scripted session action type.
	ErrUnknownSessionAction = errUtils.ErrUnknownSessionAction
	// ErrWaitTimeout indicates that a wait action did not observe the expected output before its deadline.
	ErrWaitTimeout = errUtils.ErrWaitTimeout
	// ErrUnsupportedCastKey indicates that a key action requested an unknown key sequence.
	ErrUnsupportedCastKey = errUtils.ErrUnsupportedCastKey
	// ErrScreenshotActionRequiresPath indicates a "screenshot" session action had no output path.
	ErrScreenshotActionRequiresPath = errUtils.ErrScreenshotActionRequiresPath

	errSessionProcessWaitTimeout = errors.New("timed out waiting for cast session process to exit")
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
	Path     string // Output path for a "screenshot" action.
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

	state := newSessionState(ctx, proc.output, proc.input, proc.close)
	defer state.stop()

	for i := range opts.Actions {
		if err := runAction(ctx, proc.input, state, &opts.Actions[i], opts); err != nil {
			proc.kill()
			return errors.Join(err, waitForSessionProcess(proc, defaultProcessExitMaxWait))
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

func newSessionProcessWait(wait func() error) func() error {
	done := make(chan struct{})
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			go func() {
				err = wait()
				close(done)
			}()
		})
		<-done
		return err
	}
}

type sessionState struct {
	mu     sync.Mutex
	output bytes.Buffer
	input  io.Writer
	// discardRecording suppresses the live recording stream only (toggled
	// mid-session by scripted "hide"/"show" actions); the wait-matching
	// buffer keeps updating regardless, mirroring VHS's own Hide, which only
	// suppresses recorded frames, not its internal terminal-state tracking.
	discardRecording bool
	// discardWaitBuffer additionally suppresses the wait-matching buffer; set
	// only at teardown (see discardOutput) to keep post-session exit noise
	// out of both streams.
	discardWaitBuffer bool
	changed           chan struct{}
	done              chan error
	cancel            context.CancelFunc
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
	if opts.Env == nil {
		opts.Env = map[string]string{}
	}
	if _, ok := opts.Env["PS1"]; !ok {
		opts.Env["PS1"] = defaultSessionPrompt()
	}
}

// getCurrentStyles resolves the active theme styles; a package-level var so
// tests can stub the "no styles available" fallback in defaultSessionPrompt.
var getCurrentStyles = theme.GetCurrentStyles

// defaultSessionPrompt renders a fixed "> " prompt in the same themed
// "command" style (bold, theme Primary color) that simulate-mode casts use,
// so a real shell spawned for a session-mode cast shows a prompt consistent
// with every other recording instead of leaking the shell's own default
// PS1 (which would also include the local hostname/cwd).
func defaultSessionPrompt() string {
	styles := getCurrentStyles()
	if styles == nil {
		return "> "
	}
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.TrueColor)
	renderer.SetHasDarkBackground(true)
	return styles.Command.Bold(true).Renderer(renderer).Render("> ")
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

func newSessionState(ctx context.Context, output io.Reader, input io.Writer, closeOutput func() error) *sessionState {
	watchCtx, cancel := context.WithCancel(ctx)
	state := &sessionState{
		input:   input,
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
	answerTerminalQueries(copied, s.input)
	s.mu.Lock()
	discardWaitBuffer := s.discardWaitBuffer
	if !discardWaitBuffer {
		_, _ = s.output.Write(copied)
	}
	discardRecording := s.discardRecording
	s.mu.Unlock()
	if !discardWaitBuffer {
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}
	if discardRecording {
		return
	}
	_, _ = iolib.GetContext().Data().Write(copied)
}

func answerTerminalQueries(chunk []byte, input io.Writer) {
	if input == nil || len(chunk) == 0 {
		return
	}
	if bytes.Contains(chunk, []byte("\x1b]11;?\x07")) || bytes.Contains(chunk, []byte("\x1b]11;?\x1b\\")) {
		_, _ = input.Write([]byte("\x1b]11;rgb:0000/0000/0000\x1b\\"))
	}
	if bytes.Contains(chunk, []byte("\x1b]10;?\x07")) || bytes.Contains(chunk, []byte("\x1b]10;?\x1b\\")) {
		_, _ = input.Write([]byte("\x1b]10;rgb:ffff/ffff/ffff\x1b\\"))
	}
	for i := 0; i < bytes.Count(chunk, []byte("\x1b[6n")); i++ {
		_, _ = input.Write([]byte("\x1b[1;1R"))
	}
}

func (s *sessionState) finishRead(err error) {
	if isExpectedSessionReadError(err) {
		s.done <- nil
		return
	}
	s.done <- err
}

// setDiscard toggles whether recorded output is written to the live recording
// stream, leaving the wait-matching buffer live either way. It is reversible
// (unlike the teardown-only discardOutput) so scripted "hide"/"show" session
// actions can mute and resume recording mid-session, mirroring VHS's
// Hide/Show directives: Hide only suppresses recorded frames, not internal
// terminal-state tracking, so a "Hide ... Wait ... Show" tape (hidden setup
// commands followed by a Wait for a prompt before Show) still observes hidden
// output to unblock its Wait.
func (s *sessionState) setDiscard(discard bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discardRecording = discard
}

// discardOutput suppresses both the recording stream and the wait-matching
// buffer. Used only at session teardown to keep post-exit shell noise (the
// EOT-triggered "^D...$ exit" banner) out of both.
func (s *sessionState) discardOutput() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discardRecording = true
	s.discardWaitBuffer = true
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
		waitErr := waitForSessionProcess(proc, defaultProcessExitMaxWait)
		if err == nil {
			return waitErr
		}
		if waitErr != nil {
			return errors.Join(err, waitErr)
		}
		return err
	case <-ctx.Done():
		proc.kill()
		return errors.Join(ctx.Err(), waitForSessionProcess(proc, defaultProcessExitMaxWait))
	case <-time.After(2 * time.Second):
		proc.kill()
		return waitForSessionProcess(proc, defaultProcessExitMaxWait)
	}
}

func waitForSessionProcess(proc *sessionProcess, timeout time.Duration) error {
	if proc == nil || proc.wait == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- proc.wait()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return errSessionProcessWaitTimeout
	}
}
