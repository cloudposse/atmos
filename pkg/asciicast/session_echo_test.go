package asciicast

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

func newEchoTestState(scoped bool) *sessionState {
	return &sessionState{changed: make(chan struct{}, 1), done: make(chan error, 1), scopedWaits: scoped}
}

func appendEchoTestOutput(state *sessionState, text string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.output.WriteString(text)
}

// expireEchoGrace makes every open span look like its grace period already ended.
func expireEchoGrace(state *sessionState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := range state.echo.spans {
		state.echo.spans[i].submittedAt = state.echo.spans[i].submittedAt.Add(-2 * echoNewlineGrace)
	}
}

func TestOutputMatchesSkipsSubmittedLineEcho(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		name := "unscoped"
		if scoped {
			name = "scoped"
		}
		t.Run(name, func(t *testing.T) {
			state := newEchoTestState(scoped)
			state.noteTypedWrite("sleep 3; echo READY")
			state.noteKeyPress("enter")
			appendEchoTestOutput(state, "> sleep 3; echo READY\r\n")
			if outputMatches(state, "READY", nil) {
				t.Fatal("wait matched the echo of the typed command")
			}
			if outputMatches(state, "", regexp.MustCompile(`echo READY`)) {
				t.Fatal("regex wait matched the echo of the typed command")
			}
			appendEchoTestOutput(state, "READY\r\n")
			if !outputMatches(state, "READY", nil) {
				t.Fatal("wait did not match the command output after the echo")
			}
		})
	}
}

// TestEndAnchoredPromptWaitIgnoresStalePrompt guards the common "type a command, then
// wait for the next prompt" pattern: the prompt the line was typed at must never look
// like the end of the buffer.
func TestEndAnchoredPromptWaitIgnoresStalePrompt(t *testing.T) {
	prompt := regexp.MustCompile(`\$ $`)
	for _, scoped := range []bool{false, true} {
		name := "unscoped"
		if scoped {
			name = "scoped"
		}
		t.Run(name, func(t *testing.T) {
			state := newEchoTestState(scoped)
			appendEchoTestOutput(state, "$ ")
			if !outputMatches(state, "", prompt) {
				t.Fatal("initial prompt should match before anything is typed")
			}
			state.noteTypedWrite("sleep 2; echo done")
			appendEchoTestOutput(state, "sleep 2; echo done")
			state.noteKeyPress("enter")
			if outputMatches(state, "", prompt) {
				t.Fatal("prompt wait matched the stale prompt while the echo was open")
			}
			appendEchoTestOutput(state, "\r\n")
			if outputMatches(state, "", prompt) {
				t.Fatal("prompt wait matched the stale prompt after the echo closed")
			}
			appendEchoTestOutput(state, "done\r\n$ ")
			if !outputMatches(state, "", prompt) {
				t.Fatal("prompt wait did not match the prompt printed after the command")
			}
		})
	}
}

// TestFormRedrawWhileTypingStaysMatchable guards full-screen programs: a form that
// redraws its next question while an answer is typed, before any newline arrives,
// must not have that question hidden from waits that do not match the typed answer.
func TestFormRedrawWhileTypingStaysMatchable(t *testing.T) {
	state := newEchoTestState(false)
	appendEchoTestOutput(state, "Install tools? ")
	state.noteTypedWrite("y")
	appendEchoTestOutput(state, "\x1b[7A\x1b[1mVendor component version\x1b[0m")
	state.noteKeyPress("enter")
	if !outputMatches(state, "Vendor component version", nil) {
		t.Fatal("form output redrawn while typing was hidden from a wait that does not match the typed text")
	}
}

func TestOutputMatchesScopedWaitOffsetStillAdvances(t *testing.T) {
	state := newEchoTestState(true)
	state.noteTypedWrite("echo READY")
	state.noteKeyPress("enter")
	appendEchoTestOutput(state, "> echo READY\r\nREADY\r\n")
	if !outputMatches(state, "READY", nil) {
		t.Fatal("expected match on command output")
	}
	if got, want := state.waitOffset, len("> echo READY\r\nREADY"); got != want {
		t.Fatalf("waitOffset = %d, want %d", got, want)
	}
	if outputMatches(state, "READY", nil) {
		t.Fatal("scoped wait matched already consumed output")
	}
}

func TestScopedWaitsKeepBufferedMatchesAfterNewInput(t *testing.T) {
	state := newEchoTestState(true)
	input := &syncWriter{w: io.Discard}
	appendEchoTestOutput(state, "STALE READY\r\n$ ")
	for _, action := range []SessionAction{{Type: "write", Text: "echo READY"}, {Type: "key", Key: "enter"}} {
		if err := runAction(context.Background(), input, state, &action, &SessionOptions{}); err != nil {
			t.Fatal(err)
		}
		if action.Type == "write" {
			appendEchoTestOutput(state, "echo READY")
		}
	}
	appendEchoTestOutput(state, "\r\n\x1b[32mREADY\x1b[0m READY\r\n")
	if outputMatches(state, "STALE", nil) || outputMatches(state, "echo READY", nil) {
		t.Fatal("wait matched an earlier input interval or the command echo")
	}
	for i := 0; i < 2; i++ {
		if !outputMatches(state, "READY", nil) {
			t.Fatalf("buffered match %d was lost", i+1)
		}
	}
	if outputMatches(state, "READY", nil) {
		t.Fatal("wait reused already consumed output")
	}
}

func TestOpenEchoBlocksMatchingWaitsUntilNewlineOrGrace(t *testing.T) {
	state := newEchoTestState(false)
	appendEchoTestOutput(state, "$ ")
	state.noteTypedWrite("sleep 3; echo DONE")
	appendEchoTestOutput(state, "sleep 3; echo DONE")
	state.noteKeyPress("enter")
	appendEchoTestOutput(state, "out:DONE")

	if outputMatches(state, "DONE", nil) {
		t.Fatal("an open echo must block waits that match the typed text")
	}
	if !outputMatches(state, "$ ", nil) {
		t.Fatal("waits that do not match the typed text must search as before")
	}

	expireEchoGrace(state)
	if !outputMatches(state, "out:DONE", nil) || !outputMatches(state, "DONE", nil) {
		t.Fatal("output after Enter should match once the grace period expired")
	}
	if outputMatches(state, "sleep 3", nil) {
		t.Fatal("typed text before Enter must stay unmatched after the grace period")
	}
	state.mu.Lock()
	span := state.echo.spans[0]
	state.mu.Unlock()
	if !span.closed || span.end != span.enter {
		t.Fatalf("span should close at the Enter offset, got %+v", span)
	}
}

func TestWaitForOutputRechecksWhenEchoGraceExpires(t *testing.T) {
	state := newEchoTestState(false)
	state.noteTypedWrite("run DONE")
	appendEchoTestOutput(state, "run DONE")
	state.noteKeyPress("enter")
	appendEchoTestOutput(state, "out:DONE")

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// No further output arrives, so only the grace timer can unblock the wait.
	if err := waitForOutput(ctx, state, &SessionAction{Text: "DONE", Timeout: "5s"}); err != nil {
		t.Fatalf("waitForOutput error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < echoNewlineGrace/2 {
		t.Fatalf("wait returned after %s, before the grace period", elapsed)
	}
}

func TestWriteWithoutEnterIsStillMatchable(t *testing.T) {
	state := newEchoTestState(false)
	state.noteTypedWrite("foo")
	appendEchoTestOutput(state, "foo")
	if !outputMatches(state, "foo", nil) {
		t.Fatal("waiting for the echo of an unsubmitted write must keep working")
	}
	if len(state.echo.spans) != 0 {
		t.Fatalf("unsubmitted write created spans: %+v", state.echo.spans)
	}
}

func TestBareEnterCreatesNoSpan(t *testing.T) {
	state := newEchoTestState(false)
	appendEchoTestOutput(state, "Choose one\r\n")
	state.noteKeyPress("enter")
	appendEchoTestOutput(state, "chosen\r\n")
	if len(state.echo.spans) != 0 {
		t.Fatalf("bare Enter created spans: %+v", state.echo.spans)
	}
	if !outputMatches(state, "Choose one", nil) || !outputMatches(state, "chosen", nil) {
		t.Fatal("output around a bare Enter must stay matchable")
	}
}

func TestNoteKeyPressOnlyEnterSequencesSubmit(t *testing.T) {
	tests := []struct {
		key  string
		span bool
	}{
		{key: "enter", span: true},
		{key: "Return", span: true},
		{key: " ENTER ", span: true},
		{key: "tab"},
		{key: "ctrl+c"},
		{key: "not-a-key"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			state := newEchoTestState(false)
			state.noteTypedWrite("x")
			state.noteKeyPress(tt.key)
			if got := len(state.echo.spans) == 1; got != tt.span {
				t.Fatalf("span created = %v, want %v", got, tt.span)
			}
			if got := state.echo.pending; got == tt.span {
				t.Fatalf("pending = %v after key %q", got, tt.key)
			}
		})
	}
}

func TestConsecutiveWritesExtendOnePendingLine(t *testing.T) {
	state := newEchoTestState(false)
	state.noteTypedWrite("echo ")
	appendEchoTestOutput(state, "echo ")
	state.noteTypedWrite("HI")
	appendEchoTestOutput(state, "HI")
	state.noteKeyPress("enter")
	if len(state.echo.spans) != 1 {
		t.Fatalf("spans = %+v, want exactly one", state.echo.spans)
	}
	if span := state.echo.spans[0]; span.typed != "echo HI" || span.enter != len("echo HI") {
		t.Fatalf("span = %+v, want typed %q and enter %d", span, "echo HI", len("echo HI"))
	}
	if state.echo.pending || state.echo.pendingText.Len() != 0 {
		t.Fatal("submitting a line must reset the pending line")
	}
}

func TestDisabledEchoTrackingCreatesNoSpans(t *testing.T) {
	state := newEchoTestState(false)
	state.echo.disabled = true
	state.noteTypedWrite("printf ready")
	state.noteKeyPress("enter")
	appendEchoTestOutput(state, "printf ready\r\n")
	if len(state.echo.spans) != 0 {
		t.Fatalf("disabled tracker created spans: %+v", state.echo.spans)
	}
	if !outputMatches(state, "ready", nil) {
		t.Fatal("disabled tracker must not restrict waits")
	}
}

func TestEchoTrackerSearchStart(t *testing.T) {
	contains := func(text string) func(string) bool {
		return func(candidate string) bool { return strings.Contains(candidate, text) }
	}
	tests := []struct {
		name  string
		spans []echoSpan
		from  int
		match string
		want  int
	}{
		{name: "no spans", from: 2, match: "x", want: 2},
		{name: "matching closed span moves past echo", spans: []echoSpan{{typed: "echo x", end: 7, closed: true}}, match: "x", want: 7},
		{name: "non-matching span leaves start alone", spans: []echoSpan{{typed: "echo y", end: 7, closed: true}}, match: "x", want: 0},
		{name: "matching open span blocks to end", spans: []echoSpan{{typed: "echo x", enter: 3}}, match: "x", want: 20},
		{name: "span before from is dropped", spans: []echoSpan{{typed: "echo x", end: 4, closed: true}}, from: 9, match: "x", want: 9},
		{name: "latest matching span wins", spans: []echoSpan{{typed: "x1", end: 5, closed: true}, {typed: "y", end: 9, closed: true}, {typed: "x2", end: 14, closed: true}}, match: "x", want: 14},
		{name: "empty typed text is ignored", spans: []echoSpan{{end: 8, closed: true}}, match: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := &echoTracker{spans: append([]echoSpan(nil), tt.spans...)}
			if got := tracker.searchStart(tt.from, 20, contains(tt.match)); got != tt.want {
				t.Fatalf("searchStart = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEchoTrackerResolveClosesAtFirstNewline(t *testing.T) {
	now := time.Now()
	tracker := &echoTracker{spans: []echoSpan{{typed: "cmd!", enter: 4, submittedAt: now}}}
	tracker.resolve([]byte("cmd!"), now)
	if tracker.spans[0].closed {
		t.Fatal("span closed without a newline or expired grace")
	}
	tracker.resolve([]byte("cmd!\r\nout\n"), now)
	if span := tracker.spans[0]; !span.closed || span.end != len("cmd!\r\n") {
		t.Fatalf("span = %+v, want closed at %d", span, len("cmd!\r\n"))
	}
	if _, ok := tracker.nextGrace(); ok {
		t.Fatal("closed spans must not report a grace deadline")
	}
}

func TestEchoTrackerNextGraceReturnsEarliestOpenSpan(t *testing.T) {
	first := time.Now()
	tracker := &echoTracker{spans: []echoSpan{
		{submittedAt: first.Add(time.Second)},
		{submittedAt: first},
		{submittedAt: first.Add(-time.Hour), closed: true},
	}}
	got, ok := tracker.nextGrace()
	if !ok || !got.Equal(first.Add(echoNewlineGrace)) {
		t.Fatalf("nextGrace = %v, %v; want %v", got, ok, first.Add(echoNewlineGrace))
	}
}

// TestRunSessionWaitIgnoresTypedCommandEcho drives a real session whose helper
// shell prints "out:<word>" only after a delay, while the terminal echoes the
// bare typed word immediately.
func TestRunSessionWaitIgnoresTypedCommandEcho(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatal(err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(asciicastSessionHelperEnv, "1")
	t.Setenv(asciicastSessionBannerEnv, "1")

	tests := []struct {
		name        string
		waitTimeout string
		wantTimeout bool
		minElapsed  time.Duration
	}{
		// The typed echo contains SENTINEL immediately, but the output comes
		// only after sessionHelperDelay, so a short wait must time out.
		{name: "short wait does not match echo", waitTimeout: "500ms", wantTimeout: true},
		// A long wait succeeds, but only once the command actually printed.
		{name: "long wait matches command output", waitTimeout: "10s", minElapsed: sessionHelperDelay * 2 / 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The outer context exceeds the banner wait, the delay, and the
			// wait timeout with margin so it never caps the wait early (see
			// TestRunSessionExecutesScriptedShellActions).
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			var enterAt, waitDoneAt time.Time
			err := RunSession(ctx, &SessionOptions{
				Shell: shell,
				Actions: []SessionAction{
					{Type: "wait", Text: sessionHelperBanner, Timeout: "20s"},
					{Type: "write", Text: "delay-print SENTINEL", Rate: "0"},
					{Type: "key", Key: "enter"},
					{Type: "wait", Text: "SENTINEL", Timeout: tt.waitTimeout},
				},
				Observe: func(i int, _ *SessionAction) error {
					switch i {
					case 2:
						enterAt = time.Now()
					case 3:
						waitDoneAt = time.Now()
					}
					return nil
				},
			})
			if tt.wantTimeout {
				if !errors.Is(err, ErrWaitTimeout) {
					t.Fatalf("expected wait timeout, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("RunSession error: %v", err)
			}
			if elapsed := waitDoneAt.Sub(enterAt); elapsed < tt.minElapsed {
				t.Fatalf("wait finished %s after Enter, before the command could have printed", elapsed)
			}
		})
	}
}

// TestWriteEndingInNewlineSubmitsLine covers a write whose text carries its own
// newline instead of a separate Enter key: the line is submitted, so a wait must
// not match its echo, and nothing stays pending for a later bare Enter.
func TestWriteEndingInNewlineSubmitsLine(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantSpans []string
	}{
		{name: "line feed", text: "sleep 3; echo READY\n", wantSpans: []string{"sleep 3; echo READY"}},
		{name: "carriage return", text: "sleep 3; echo READY\r", wantSpans: []string{"sleep 3; echo READY"}},
		{name: "crlf counts once", text: "sleep 3; echo READY\r\n", wantSpans: []string{"sleep 3; echo READY"}},
		{name: "two lines", text: "echo ONE\necho READY\n", wantSpans: []string{"echo ONE", "echo READY"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newEchoTestState(false)
			action := &SessionAction{Type: "write", Text: tt.text, Rate: "0"}
			if err := runTypedWrite(t.Context(), &syncWriter{w: io.Discard}, state, action, 0); err != nil {
				t.Fatalf("runTypedWrite error: %v", err)
			}
			if len(state.echo.spans) != len(tt.wantSpans) {
				t.Fatalf("spans = %+v, want %d", state.echo.spans, len(tt.wantSpans))
			}
			for i, want := range tt.wantSpans {
				if got := state.echo.spans[i].typed; got != want {
					t.Fatalf("span %d typed = %q, want %q", i, got, want)
				}
			}
			if state.echo.pending || state.echo.pendingText.Len() != 0 {
				t.Fatal("a submitted line must not stay pending")
			}

			appendEchoTestOutput(state, "> sleep 3; echo READY\r\n")
			if outputMatches(state, "READY", nil) {
				t.Fatal("wait matched the echo of a line submitted by its own newline")
			}
			appendEchoTestOutput(state, "READY\r\n")
			if !outputMatches(state, "READY", nil) {
				t.Fatal("wait did not match the command output after the echo")
			}

			// A later bare Enter (for example confirming a prompt) records nothing.
			state.noteKeyPress("enter")
			if len(state.echo.spans) != len(tt.wantSpans) {
				t.Fatalf("bare Enter after a submitted write created a span: %+v", state.echo.spans)
			}
		})
	}
}

func TestRunTypedWriteSendsTextUnchanged(t *testing.T) {
	var buf bytes.Buffer
	state := newEchoTestState(false)
	text := "echo ONE\r\necho TWO\npartial"
	if err := runTypedWrite(t.Context(), &syncWriter{w: &buf}, state, &SessionAction{Type: "write", Text: text, Rate: "0"}, 0); err != nil {
		t.Fatalf("runTypedWrite error: %v", err)
	}
	if got := buf.String(); got != text {
		t.Fatalf("written = %q, want %q", got, text)
	}
	if !state.echo.pending || state.echo.pendingText.String() != "partial" {
		t.Fatalf("unterminated tail should stay pending, got pending=%v text=%q", state.echo.pending, state.echo.pendingText.String())
	}
}

func TestRunTypedWriteRejectsInvalidRate(t *testing.T) {
	state := newEchoTestState(false)
	for _, text := range []string{"", "x\n"} {
		err := runTypedWrite(t.Context(), &syncWriter{w: io.Discard}, state, &SessionAction{Type: "write", Text: text, Rate: "fast"}, 0)
		if err == nil || !strings.Contains(err.Error(), "invalid write rate") {
			t.Fatalf("text %q: expected invalid write rate error, got %v", text, err)
		}
	}
}

func TestPlanTypedSegments(t *testing.T) {
	tests := []struct {
		text string
		want []typedSegment
	}{
		{text: "echo hi", want: []typedSegment{{text: "echo hi"}}},
		{text: "echo hi\n", want: []typedSegment{{text: "echo hi"}, {text: "\n", submitted: true}}},
		{text: "echo hi\r\n", want: []typedSegment{{text: "echo hi"}, {text: "\r\n", submitted: true}}},
		{text: "a\rb", want: []typedSegment{{text: "a"}, {text: "\r", submitted: true}, {text: "b"}}},
		{text: "\n\n", want: []typedSegment{{text: "\n", submitted: true}, {text: "\n", submitted: true}}},
	}
	for _, tt := range tests {
		t.Run(strconv.Quote(tt.text), func(t *testing.T) {
			got := planTypedSegments(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("planTypedSegments(%q) = %+v, want %+v", tt.text, got, tt.want)
			}
		})
	}
}

// firstWriteSignal records writes and closes started on the first one.
type firstWriteSignal struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	started chan struct{}
	once    sync.Once
}

func (w *firstWriteSignal) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.once.Do(func() { close(w.started) })
	return w.buf.Write(p)
}

// TestRunTypedWriteIsAtomic guards against a terminal-query response landing
// between a command's text and its newline: a concurrent writer that starts
// mid-write must wait until the whole multi-line text has been sent.
func TestRunTypedWriteIsAtomic(t *testing.T) {
	sink := &firstWriteSignal{started: make(chan struct{})}
	input := &syncWriter{w: sink}
	state := newEchoTestState(false)
	text := "echo ONE\necho TWO\n"

	done := make(chan error, 1)
	go func() {
		done <- runTypedWrite(t.Context(), input, state, &SessionAction{Type: "write", Text: text, Rate: "2ms"}, 0)
	}()
	<-sink.started
	if _, err := input.Write([]byte("QUERY-RESPONSE")); err != nil {
		t.Fatalf("concurrent write: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("runTypedWrite error: %v", err)
	}
	sink.mu.Lock()
	got := sink.buf.String()
	sink.mu.Unlock()
	if want := text + "QUERY-RESPONSE"; got != want {
		t.Fatalf("written = %q, want %q (the concurrent write must not interleave)", got, want)
	}
	if len(state.echo.spans) != 2 {
		t.Fatalf("spans = %+v, want two submitted lines", state.echo.spans)
	}
}
