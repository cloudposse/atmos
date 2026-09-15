package step

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/schema"
)

// This file implements the mode: session translator: turning a parsed
// tape's directives into SessionAction-shaped children (write/key/pause/
// wait/hide/show/screenshot). cast_tape.go owns directive parsing, setup-
// command tracking, and the separate mode: steps translator.

// tapeTypedCommand is one `Type "cmd" [Sleep <dur>] Enter` idiom recognized
// as a single logical typed command.
type tapeTypedCommand struct {
	Text        string
	SleepBefore string
	Consumed    int // Number of directives consumed, always >= 1.
	HasEnter    bool
}

// tapeConsumeTypedCommand recognizes the `Type "cmd" [Sleep <dur>] Enter`
// idiom as one logical typed command by looking ahead from a TapeType
// directive at index i for an optional immediately-following TapeSleep and a
// TapeKey{enter}.
func tapeConsumeTypedCommand(directives []asciicast.TapeDirective, i int) tapeTypedCommand {
	cmd := tapeTypedCommand{Text: directives[i].Value, Consumed: 1}
	next := i + 1
	if next < len(directives) && directives[next].Kind == asciicast.TapeSleep {
		cmd.SleepBefore = directives[next].Duration
		cmd.Consumed++
		next++
	}
	if next < len(directives) && directives[next].Kind == asciicast.TapeKey && directives[next].Key == "enter" {
		cmd.HasEnter = true
		cmd.Consumed++
	}
	return cmd
}

// tapeSessionTranslator holds the mutable state translateTapeSession's
// directive-by-directive dispatch needs (the emitted steps, the buffered
// contents of the current Hide ... Show region, and whether one is active),
// so dispatch() and its per-directive handlers can each stay a small,
// single-responsibility method instead of one large function closing over
// shared local variables.
type tapeSessionTranslator struct {
	step  *schema.WorkflowStep
	state *tapeTrackedState
	steps []schema.WorkflowStep
	// hiddenBuffer accumulates actions found inside the current Hide ... Show
	// region. It's only flushed (wrapped in hide/show) at Show if it actually
	// ended up non-empty -- when every Type inside the region matched a
	// recognized setup command, nothing is buffered, so no hide/show pair is
	// emitted at all: there's nothing to mute in the first place.
	hiddenBuffer []schema.WorkflowStep
	hidden       bool
}

// translateTapeSession converts a tape's directives into mode: session
// SessionAction-shaped children (write/key/pause/wait/hide/show/screenshot).
// Type ... Enter sequences inside a Hide ... Show region are pattern-matched
// against tapeSetupCommand; a match lifts it into tracked cwd/env state
// instead of being replayed into the PTY (avoiding the discard-toggle path
// for the common case); anything unmatched inside Hide falls back to literal
// write/key replay, so nothing inside a real tape's Hide block is silently
// dropped.
func translateTapeSession(step *schema.WorkflowStep, directives []asciicast.TapeDirective, state *tapeTrackedState) ([]schema.WorkflowStep, error) {
	t := &tapeSessionTranslator{step: step, state: state}
	i := 0
	for i < len(directives) {
		next, err := t.dispatch(directives, i)
		if err != nil {
			return nil, err
		}
		i = next
	}
	// A Hide with no matching Show (malformed tape, or truncated at EOF):
	// flush whatever was buffered so nothing is silently dropped.
	if len(t.hiddenBuffer) > 0 {
		t.steps = append(t.steps, schema.WorkflowStep{Type: "hide"})
		t.steps = append(t.steps, t.hiddenBuffer...)
	}
	return t.steps, nil
}

// dispatch handles the directive at index i and returns the next index to
// process. Cases with their own branching (Hide/Show/Sleep/Wait/Type) stay
// here; single-action or no-op cases delegate to dispatchSimple, keeping
// each switch's cyclomatic complexity low instead of accumulating both
// groups into one function.
func (t *tapeSessionTranslator) dispatch(directives []asciicast.TapeDirective, i int) (int, error) {
	d := &directives[i]
	switch d.Kind {
	case asciicast.TapeHide:
		t.hidden = true
		return i + 1, nil
	case asciicast.TapeShow:
		t.handleShow()
		return i + 1, nil
	case asciicast.TapeSleep:
		t.emitPacing(&schema.WorkflowStep{Type: "pause", Duration: d.Duration})
		return i + 1, nil
	case asciicast.TapeWait:
		if err := t.handleWait(d); err != nil {
			return 0, err
		}
		return i + 1, nil
	case asciicast.TapeType:
		return t.handleType(directives, i)
	default:
		return t.dispatchSimple(d, i)
	}
}

// dispatchSimple handles directive kinds with no branching of their own:
// Set/Output/Require/Source were already applied by applyTapeSettings and
// are no-ops here, Screenshot/Key each emit exactly one step, and
// Copy/Paste/Env have no mode: session equivalent.
func (t *tapeSessionTranslator) dispatchSimple(d *asciicast.TapeDirective, i int) (int, error) {
	switch d.Kind {
	case asciicast.TapeScreenshot:
		t.emit(&schema.WorkflowStep{Type: schema.TaskTypeScreenshot, Path: d.Value})
	case asciicast.TapeKey:
		t.emit(&schema.WorkflowStep{Type: "key", Key: d.Key, Repeat: d.Repeat})
	case asciicast.TapeCopy, asciicast.TapePaste, asciicast.TapeEnv:
		return 0, tapeDirectiveError(d, fmt.Errorf("%w: rewrite the tape before using tape or tape_file", ErrUnsupportedTapeDirective))
	}
	return i + 1, nil
}

func (t *tapeSessionTranslator) emit(s *schema.WorkflowStep) {
	if t.hidden {
		t.hiddenBuffer = append(t.hiddenBuffer, *s)
		return
	}
	t.steps = append(t.steps, *s)
}

// emitPacing routes a Sleep/Wait directly to the top-level, visible action
// list when nothing has fallen back to literal hidden replay yet
// (hiddenBuffer still empty): every setup command so far was lifted, so this
// pacing action isn't synchronizing with muted PTY activity -- it's really
// about giving the freshly-spawned shell (started with the lifted env/cwd
// already applied) a moment to become ready, which matters whether or not
// anything is nominally "hidden" at this point, so it stays a plain visible
// action rather than being wrapped in a hide/show pair with nothing else in
// it. Once real hidden replay has occurred, a Sleep/Wait genuinely
// paces/synchronizes with that hidden activity and stays grouped inside the
// hidden buffer instead.
func (t *tapeSessionTranslator) emitPacing(s *schema.WorkflowStep) {
	if t.hidden && len(t.hiddenBuffer) == 0 {
		t.steps = append(t.steps, *s)
		return
	}
	t.emit(s)
}

func (t *tapeSessionTranslator) handleShow() {
	t.hidden = false
	if len(t.hiddenBuffer) > 0 {
		t.steps = append(t.steps, schema.WorkflowStep{Type: "hide"})
		t.steps = append(t.steps, t.hiddenBuffer...)
		t.steps = append(t.steps, schema.WorkflowStep{Type: "show"})
	}
	t.hiddenBuffer = nil
}

func (t *tapeSessionTranslator) handleWait(d *asciicast.TapeDirective) error {
	if d.Regex == "" {
		return tapeDirectiveError(d, fmt.Errorf(
			"%w: bare Wait (wait for terminal idle) has no mode: session equivalent; add a pattern, e.g. Wait /prompt$/",
			ErrUnsupportedTapeDirective,
		))
	}
	t.emitPacing(&schema.WorkflowStep{Type: "wait", Regex: d.Regex, Timeout: t.step.Timeout})
	return nil
}

func (t *tapeSessionTranslator) handleType(directives []asciicast.TapeDirective, i int) (int, error) {
	cmd := tapeConsumeTypedCommand(directives, i)
	next := i + cmd.Consumed
	if t.hidden && cmd.HasEnter {
		if setup := matchTapeSetupCommand(cmd.Text); setup.Kind != "" {
			t.state.apply(setup)
			return next, nil
		}
	}
	t.emit(&schema.WorkflowStep{Type: "write", Text: cmd.Text})
	if cmd.SleepBefore != "" {
		t.emit(&schema.WorkflowStep{Type: "pause", Duration: cmd.SleepBefore})
	}
	if cmd.HasEnter {
		t.emit(&schema.WorkflowStep{Type: "key", Key: "enter"})
	}
	return next, nil
}
