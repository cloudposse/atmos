package asciicast

import (
	"bytes"
	"runtime"
	"strings"
	"time"
)

// This file keeps scripted "wait" actions from matching the terminal's echo of
// a line the script itself typed and submitted. A PTY echoes typed input back
// into the same output stream a wait scans, so without this a script that
// types `sleep 3; echo READY`, presses Enter, and waits for READY would be
// satisfied by the echo of its own command instead of by the command's output.
//
// The rule is: a wait never matches the echo of a line typed with "write"
// actions and submitted with an Enter "key" action. When a wait's text or regex
// matches what was typed on a submitted line, the wait searches only output
// after that line's echo, which ends at the first newline following the Enter.
// Waits that do not match any typed line search exactly as before, so output
// redrawn by full-screen programs while a line is typed (for example the next
// question of a form) stays matchable, and the buffer is never cut, so
// end-anchored patterns such as `\$ $` cannot see a stale prompt as the end of
// the output. Writes without a following Enter (for example a script that types
// text and waits for the typed text itself) never restrict a wait, and a bare
// Enter with nothing typed records no line.

// echoNewlineGrace is how long a submitted line's echo stays open while waiting
// for its echoed newline. Programs with echo disabled never emit one, so after
// the grace period the echo is treated as ending where Enter was sent.
const echoNewlineGrace = 500 * time.Millisecond

// echoSpan is one typed-and-submitted line whose terminal echo a matching wait must skip.
// Offsets are raw byte offsets into sessionState.output.
type echoSpan struct {
	// typed is the text the script typed on the line.
	typed string
	// enter is the output offset taken just before the Enter bytes were written.
	enter int
	// submittedAt is when Enter was sent; the grace period is measured from it.
	submittedAt time.Time
	// end is the exclusive end of the line's echo once closed.
	end int
	// closed reports whether end is final. An open span's echo runs to the end of the buffer.
	closed bool
}

// echoTracker records typed-line echo spans. Every method requires the owning
// sessionState.mu to be held by the caller.
type echoTracker struct {
	// disabled turns tracking off for sessions that have no terminal echo.
	disabled bool
	// pending reports whether a typed line has started but not yet been submitted.
	pending bool
	// pendingText accumulates the text typed on the pending line.
	pendingText strings.Builder
	spans       []echoSpan
}

// echoTrackingSupported reports whether the session's shell echoes typed input.
// Windows sessions use plain pipes rather than a PTY, so nothing is echoed and
// the first line of real output would otherwise be mistaken for an echo.
func echoTrackingSupported() bool {
	return runtime.GOOS != "windows"
}

// noteWrite records text about to be typed on the pending line.
func (t *echoTracker) noteWrite(text string) {
	if t.disabled {
		return
	}
	t.pending = true
	t.pendingText.WriteString(text)
}

// noteEnter records that Enter is about to be sent at output offset outLen. It
// creates a span only when a typed line is pending.
func (t *echoTracker) noteEnter(outLen int, now time.Time) {
	if t.disabled || !t.pending {
		return
	}
	t.pending = false
	t.spans = append(t.spans, echoSpan{typed: t.pendingText.String(), enter: outLen, submittedAt: now})
	t.pendingText.Reset()
}

// resolve closes spans whose echoed newline arrived or whose grace period expired.
func (t *echoTracker) resolve(output []byte, now time.Time) {
	for i := range t.spans {
		span := &t.spans[i]
		if span.closed {
			continue
		}
		if span.enter <= len(output) {
			if idx := bytes.IndexByte(output[span.enter:], '\n'); idx >= 0 {
				span.end = span.enter + idx + 1
				span.closed = true
				continue
			}
		}
		if now.Sub(span.submittedAt) >= echoNewlineGrace {
			span.end = span.enter
			span.closed = true
		}
	}
}

// nextGrace returns the earliest time an open span's grace period expires.
func (t *echoTracker) nextGrace() (time.Time, bool) {
	var earliest time.Time
	found := false
	for i := range t.spans {
		if t.spans[i].closed {
			continue
		}
		deadline := t.spans[i].submittedAt.Add(echoNewlineGrace)
		if !found || deadline.Before(earliest) {
			earliest = deadline
			found = true
		}
	}
	return earliest, found
}

// searchStart returns where a wait may start searching output. It is from,
// moved past the echo of every submitted line whose typed text the wait itself
// matches; an open echo moves it to the end of the buffer until the echo closes.
// Closed spans that end at or before from can never matter again and are dropped.
func (t *echoTracker) searchStart(from, outputLen int, matches func(string) bool) int {
	from = min(from, outputLen)
	t.dropSpansBefore(from)
	for _, span := range t.spans {
		if span.typed != "" && matches(span.typed) {
			from = max(from, span.effectiveEnd(outputLen))
		}
	}
	return from
}

// dropSpansBefore forgets closed spans that end at or before offset.
func (t *echoTracker) dropSpansBefore(offset int) {
	kept := t.spans[:0]
	for _, span := range t.spans {
		if span.closed && span.end <= offset {
			continue
		}
		kept = append(kept, span)
	}
	t.spans = kept
}

// effectiveEnd returns where the span's echo ends: an open span's echo runs to the end of the buffer.
func (s echoSpan) effectiveEnd(outputLen int) int {
	if !s.closed || s.end > outputLen {
		return outputLen
	}
	return s.end
}

// noteTypedWrite is called before a "write" action types text.
func (s *sessionState) noteTypedWrite(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.echo.noteWrite(text)
}

// noteKeyPress is called before a "key" action sends its bytes; an Enter key
// submits the pending typed line.
func (s *sessionState) noteKeyPress(key string) {
	if s == nil {
		return
	}
	seq, err := keySequence(key)
	if err != nil || seq != "\r" {
		return
	}
	s.noteSubmittedLine()
}

// noteSubmittedLine is called just before the bytes that submit the pending
// typed line are sent: an Enter key, or a newline inside written text.
func (s *sessionState) noteSubmittedLine() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.echo.noteEnter(s.output.Len(), time.Now())
}

// nextEchoGrace returns when waits must re-evaluate because an open span's grace period ends.
func (s *sessionState) nextEchoGrace() (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.echo.nextGrace()
}

// graceTimer wakes waitForOutput when an echo span's grace period expires
// without any new output arriving.
type graceTimer struct {
	timer *time.Timer
}

// arm re-arms the timer for the earliest open span's grace deadline and returns
// its channel, or nil when no span is open.
func (g *graceTimer) arm(state *sessionState) <-chan time.Time {
	deadline, ok := state.nextEchoGrace()
	if !ok {
		return nil
	}
	wait := time.Until(deadline)
	if g.timer == nil {
		g.timer = time.NewTimer(wait)
	} else {
		resetTimer(g.timer, wait)
	}
	return g.timer.C
}

func (g *graceTimer) stop() {
	if g.timer != nil {
		g.timer.Stop()
	}
}
