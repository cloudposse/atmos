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
	// Fallback safety valve for finishSession when a shell never signals
	// exit via its output stream (the `done` channel) or the caller's
	// context. Real interactive shells -- notably bash with
	// profile-loading overhead (.bashrc/.bash_profile, history flushing
	// on exit) -- can legitimately take longer than a couple of seconds
	// to fully process EOT and exit, especially on first startup; too
	// tight a value here forcibly kills a shell that was about to exit
	// cleanly on its own, discarding an otherwise-successful recording.
	defaultSessionExitMaxWait = 5 * time.Second
	sessionReadBufferSize     = 4096
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
	// ErrSimulateActionMissingCallback indicates a "simulate" action was built without its Fn callback set.
	ErrSimulateActionMissingCallback = errUtils.ErrSimulateActionMissingCallback

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
	// Fn runs a caller-supplied action (Type == "simulate") in place, letting
	// a session mix in the same styled, non-interactive narration steps
	// mode: steps uses (see pkg/runner/step's simulate rendering) instead of
	// typing raw, unstyled keystrokes for comment/narration lines. asciicast
	// deliberately has no styling logic of its own; the caller renders and
	// writes the styled bytes itself, and this callback is how a session
	// action list carries that back out to it in order.
	Fn func() error
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
	// The PTY input is written from two independent goroutines: the
	// background output-reader (answerTerminalQueries, whenever the shell
	// emits a terminal capability query) and this function's own action
	// loop/teardown EOT write below. Neither pty writes nor Go's io.Writer
	// contract guarantee atomicity across concurrent, unsynchronized
	// callers, so without serializing them a query response can interleave
	// mid-write with a scripted "write"/"key" action and corrupt the
	// command the shell receives. syncedInput protects every subsequent use
	// of the pty input (newSessionState, runAction, finishSession via
	// proc.input), and its Locked method lets a whole multi-write action
	// (every character of a "write", every response byte of a query answer)
	// stay atomic as a unit rather than just each individual Write call.
	syncedInput := &syncWriter{w: proc.input}
	proc.input = syncedInput

	state := newSessionState(ctx, proc.output, syncedInput, proc.close)
	defer state.stop()

	for i := range opts.Actions {
		if err := runAction(ctx, syncedInput, state, &opts.Actions[i], opts); err != nil {
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

// syncWriter serializes concurrent Write calls to a shared io.WriteCloser.
// See its construction site in RunSession for why this is required: the PTY
// input is written from more than one goroutine, and neither pty writes nor
// io.Writer implementations are guaranteed atomic across concurrent,
// unsynchronized callers.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	defer perf.Track(nil, "asciicast.syncWriter.Write")()

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// Close closes the wrapped writer if it supports it; a no-op otherwise, so
// syncWriter can wrap any io.Writer (e.g. io.Discard in tests) while still
// satisfying io.WriteCloser for the real session's PTY input.
func (s *syncWriter) Close() error {
	defer perf.Track(nil, "asciicast.syncWriter.Close")()

	if c, ok := s.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Locked runs fn while holding the writer's lock, so every Write fn issues
// is atomic as a whole relative to any other writer sharing this
// syncWriter -- e.g. every character of one scripted "write" action, or
// every response byte of one terminal-query answer, stays together as an
// indivisible unit instead of being split apart by a concurrent writer
// landing between two of its individual Write calls.
func (s *syncWriter) Locked(fn func(io.Writer)) {
	defer perf.Track(nil, "asciicast.syncWriter.Locked")()

	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s.w)
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
	input  *syncWriter
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
	// pendingQueryTail holds a trailing fragment of the previous output
	// chunk that looks like the start of a terminal-capability query
	// (e.g. "\x1b]10;?") but was cut off before its terminator by a PTY
	// read boundary. answerTerminalQueries only scans one chunk at a time
	// via bytes.Contains, so without carrying this forward and prepending
	// it to the next chunk, a query split across two reads is silently
	// never answered -- observed with real bash (whose query writes seem
	// more likely to land in separate reads than zsh/sh's), which then
	// retries the same query indefinitely, corrupting/hanging the session.
	pendingQueryTail []byte
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

func newSessionState(ctx context.Context, output io.Reader, input *syncWriter, closeOutput func() error) *sessionState {
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
	// Query detection scans pendingQueryTail (carried over from the previous
	// chunk) plus this chunk together, so a query split across a PTY read
	// boundary is still recognized -- see the pendingQueryTail field comment.
	// The recorded/wait-matching streams below still only ever see `copied`
	// (the carried-over prefix was already recorded when it first arrived).
	scan := copied
	if len(s.pendingQueryTail) > 0 {
		scan = append(append([]byte(nil), s.pendingQueryTail...), copied...)
	}
	// The whole query-response burst (background color, cursor position,
	// etc. -- answerTerminalQueries can issue several Write calls for one
	// chunk) must land as one atomic unit, or a concurrent scripted action
	// could interleave between two of its individual writes. input is nil
	// in tests that only care about output capture, not query-answering.
	if s.input != nil {
		s.input.Locked(func(w io.Writer) { answerTerminalQueries(scan, w) })
	}
	s.pendingQueryTail = terminalQueryTail(scan)
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

// terminalQueryPatterns lists every exact terminal-capability query byte
// sequence answerTerminalQueries recognizes and answers.
var terminalQueryPatterns = [][]byte{
	[]byte("\x1b]11;?\x07"),
	[]byte("\x1b]11;?\x1b\\"),
	[]byte("\x1b]10;?\x07"),
	[]byte("\x1b]10;?\x1b\\"),
	[]byte("\x1b[6n"),
}

// maxTerminalQueryPatternLen is the length of the longest entry in
// terminalQueryPatterns.
var maxTerminalQueryPatternLen = func() int {
	max := 0
	for _, p := range terminalQueryPatterns {
		if len(p) > max {
			max = len(p)
		}
	}
	return max
}()

// terminalQueryTail returns the longest suffix of chunk that is a proper
// (strictly shorter) prefix of one of terminalQueryPatterns -- i.e. bytes
// that could be the start of a terminal-capability query cut off by a PTY
// read boundary, worth carrying into the next chunk. Returns nil if chunk's
// tail doesn't look like the start of any recognized query.
func terminalQueryTail(chunk []byte) []byte {
	limit := maxTerminalQueryPatternLen - 1
	if limit > len(chunk) {
		limit = len(chunk)
	}
	for length := limit; length > 0; length-- {
		tail := chunk[len(chunk)-length:]
		for _, pattern := range terminalQueryPatterns {
			if len(tail) < len(pattern) && bytes.Equal(pattern[:len(tail)], tail) {
				return append([]byte(nil), tail...)
			}
		}
	}
	return nil
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
	case <-time.After(defaultSessionExitMaxWait):
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
