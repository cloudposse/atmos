package step

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	ErrCastTapeSourceConflict           = errUtils.ErrCastTapeSourceConflict
	ErrCastTapeStepsConflict            = errUtils.ErrCastTapeStepsConflict
	ErrTapeDirectiveRequiresSessionMode = errUtils.ErrTapeDirectiveRequiresSessionMode
	ErrUnsupportedTapeOutputExtension   = errUtils.ErrUnsupportedTapeOutputExtension
	ErrUnsupportedTapeDirective         = errUtils.ErrUnsupportedTapeDirective
)

// osTapeFileReader reads a tape file (or a Source-referenced file) from real
// disk. It is the only production implementer of asciicast.TapeFileReader;
// tests inject a fake instead.
type osTapeFileReader struct{}

func (osTapeFileReader) ReadFile(path string) ([]byte, error) {
	defer perf.Track(nil, "step.osTapeFileReader.ReadFile")()

	return os.ReadFile(path)
}

// expandCastTape interprets step.Tape/step.TapeFile (if either is set) into
// step.Steps, seeding step.Shell/WriteRate/CastOutput/Tools from the tape's
// Set/Output/Require directives (explicit step-level fields always win over
// a tape-implied value). It is a no-op when neither Tape nor TapeFile is set.
// Called once from CastHandler.Validate, before mode-specific validation, so
// hand-authored steps: and tape-derived steps: are indistinguishable to
// everything downstream.
func expandCastTape(step *schema.WorkflowStep) error {
	if step.Tape == "" && step.TapeFile == "" {
		return nil
	}
	if err := validateCastTapeConflicts(step); err != nil {
		return err
	}

	tape, err := loadTape(step)
	if err != nil {
		return err
	}
	if err := applyTapeSettings(step, tape.Directives); err != nil {
		return err
	}

	state := newTapeTrackedState(step)
	children, err := translateTapeByMode(step, tape, state)
	if err != nil {
		return err
	}
	step.Steps = children
	return nil
}

// validateCastTapeConflicts rejects tape/tape_file combined with each other,
// or with hand-authored steps:, before any parsing is attempted.
func validateCastTapeConflicts(step *schema.WorkflowStep) error {
	if step.Tape != "" && step.TapeFile != "" {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastTapeSourceConflict, step.Name)
	}
	if len(step.Steps) > 0 {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastTapeStepsConflict, step.Name)
	}
	return nil
}

// translateTapeByMode dispatches a parsed tape's directives to the
// mode-appropriate translator, seeding step.Env/WorkingDirectory from the
// tracked session state whenever mode: session is in play (even on error,
// matching the original inline behavior -- the caller aborts on a non-nil
// error before step.Steps is ever assigned, so this is inconsequential).
func translateTapeByMode(step *schema.WorkflowStep, tape *asciicast.Tape, state *tapeTrackedState) ([]schema.WorkflowStep, error) {
	switch castMode(step) {
	case "steps":
		return translateTapeSteps(step.Name, tape.Directives, state)
	case "session":
		children, err := translateTapeSession(step, tape.Directives, state)
		step.Env = state.env
		step.WorkingDirectory = state.workingDirectory
		return children, err
	default:
		return nil, fmt.Errorf(wrappedQuotedErrorFormat, ErrInvalidCastMode, step.Mode)
	}
}

func loadTape(step *schema.WorkflowStep) (*asciicast.Tape, error) {
	baseDir := tapeBaseDir(step)
	if step.TapeFile != "" {
		tapeFile := step.TapeFile
		if !filepath.IsAbs(tapeFile) {
			tapeFile = filepath.Join(baseDir, tapeFile)
		}
		return asciicast.ParseTapeFile(tapeFile, baseDir, osTapeFileReader{})
	}
	return asciicast.ParseTape(step.Tape, "", baseDir, osTapeFileReader{})
}

// tapeBaseDir resolves the directory every Source directive resolves
// relative to, for both tape: and tape_file: -- the step's own
// working_directory if set, else the process CWD. This is the SAME base for
// tape_file: as for an inline tape: block; it is deliberately not
// filepath.Dir(step.TapeFile), matching real VHS, which resolves every
// Source in a script relative to its own single invocation CWD, not
// relative to the directory of whichever file a Source happened to be read
// from (see the loadSource doc comment in pkg/asciicast/tape.go for how this
// was confirmed against this repo's own demo/landing/*.tape).
func tapeBaseDir(step *schema.WorkflowStep) string {
	if step.WorkingDirectory != "" {
		return step.WorkingDirectory
	}
	return "."
}

func tapeDirectiveError(d *asciicast.TapeDirective, err error) error {
	return &asciicast.TapeError{File: d.File, Line: d.Line, Err: err}
}

// tapeDirectiveKindName renders a TapeDirectiveKind as its VHS keyword, for
// log messages naming which directive was skipped.
func tapeDirectiveKindName(kind asciicast.TapeDirectiveKind) string {
	switch kind {
	case asciicast.TapeSleep:
		return "Sleep"
	case asciicast.TapeWait:
		return "Wait"
	default:
		return "directive"
	}
}

// tapeCosmeticSetKeys are VHS Set keys with no Atmos rendering analog
// (video/GIF chrome Atmos's renderer doesn't produce) plus WaitPattern (out
// of scope for v1 -- every real tape in this repo always repeats the pattern
// explicitly on each Wait line). Silently ignored, not an error.
var tapeCosmeticSetKeys = map[string]bool{
	"FontFamily": true, "FontSize": true, "Theme": true, "Margin": true,
	"Padding": true, "BorderRadius": true, "Framerate": true, "CursorBlink": true,
	"LetterSpacing": true, "LineHeight": true, "MarginFill": true, "WindowBar": true,
	"PlaybackSpeed": true, "LoopOffset": true, "WaitPattern": true,
}

// applyTapeSettings applies every Set/Output/Require directive onto step,
// with "explicit step field wins" precedence for Set/Output. Screenshot,
// Hide/Show, Type, Sleep, Wait, and keypress directives are handled by the
// mode-specific translators, not here.
func applyTapeSettings(step *schema.WorkflowStep, directives []asciicast.TapeDirective) error {
	for i := range directives {
		d := &directives[i]
		switch d.Kind {
		case asciicast.TapeSet:
			applyTapeSetDirective(step, d)
		case asciicast.TapeOutput:
			if err := applyTapeOutputDirective(step, d); err != nil {
				return err
			}
		case asciicast.TapeRequire:
			appendTapeTool(step, d.Value)
		}
	}
	return nil
}

func applyTapeSetDirective(step *schema.WorkflowStep, d *asciicast.TapeDirective) {
	switch d.Key {
	case "Shell":
		if step.Shell == "" {
			step.Shell = d.Value
		}
	case "Width":
		applyTapeSetInt(&step.Width, d.Value)
	case "Height":
		applyTapeSetInt(&step.Height, d.Value)
	case "TypingSpeed":
		if step.WriteRate == "" {
			step.WriteRate = d.Value
		}
	case "WaitTimeout":
		if step.Timeout == "" {
			step.Timeout = d.Value
		}
	default:
		logUnsupportedTapeSet(step, d)
	}
}

// applyTapeSetInt fills current from value if current is still unset (<= 0)
// and value parses as an integer; a malformed or already-explicit value is
// left untouched.
func applyTapeSetInt(current *int, value string) {
	if *current > 0 {
		return
	}
	if n, err := strconv.Atoi(value); err == nil {
		*current = n
	}
}

func logUnsupportedTapeSet(step *schema.WorkflowStep, d *asciicast.TapeDirective) {
	reason := "unrecognized Set key"
	if tapeCosmeticSetKeys[d.Key] {
		reason = "no Atmos rendering equivalent for this Set key"
	}
	log.Warn("tape: Set directive not supported, skipping",
		"reason", reason, "key", d.Key, "value", d.Value, "step", step.Name, "file", d.File, "line", d.Line)
}

// tapeOutputExtensions maps a lowercase file extension onto the CastOutput
// field it should seed, matching atmos cast render's own supported-format
// list exactly (stricter than VHS's own renderer support -- notably .webm,
// used by every demo/landing/*.tape Output line today, is not in this list).
var tapeOutputExtensions = map[string]func(*schema.CastOutput, string){
	".cast": func(o *schema.CastOutput, p string) {
		if o.Cast == "" {
			o.Cast = p
		}
	},
	".gif": func(o *schema.CastOutput, p string) {
		if o.GIF == "" {
			o.GIF = p
		}
	},
	".mp4": func(o *schema.CastOutput, p string) {
		if o.MP4 == "" {
			o.MP4 = p
		}
	},
	".html": func(o *schema.CastOutput, p string) {
		if o.HTML == "" {
			o.HTML = p
		}
	},
	".ascii": func(o *schema.CastOutput, p string) {
		if o.ASCII == "" {
			o.ASCII = p
		}
	},
	".png": func(o *schema.CastOutput, p string) {
		if o.PNG == "" {
			o.PNG = p
		}
	},
	".jpg": func(o *schema.CastOutput, p string) {
		if o.JPG == "" {
			o.JPG = p
		}
	},
	".jpeg": func(o *schema.CastOutput, p string) {
		if o.JPG == "" {
			o.JPG = p
		}
	},
}

func applyTapeOutputDirective(step *schema.WorkflowStep, d *asciicast.TapeDirective) error {
	ext := strings.ToLower(filepath.Ext(d.Value))
	apply, ok := tapeOutputExtensions[ext]
	if !ok {
		return tapeDirectiveError(d, fmt.Errorf("%w: %s", ErrUnsupportedTapeOutputExtension, d.Value))
	}
	if step.CastOutput == nil {
		step.CastOutput = &schema.CastOutput{}
	}
	apply(step.CastOutput, d.Value)
	return nil
}

func appendTapeTool(step *schema.WorkflowStep, tool string) {
	for _, existing := range step.Tools {
		if existing == tool {
			return
		}
	}
	step.Tools = append(step.Tools, tool)
}

// tapeSetupCommand is a recognized shell idiom inside a Type directive's text
// that VHS authors use purely to establish state (env vars, cwd) before the
// visible/functional part of a demo -- Atmos has native env:/working_directory:
// step config for exactly this, so these are lifted into tracked state
// instead of being replayed as literal typed/executed commands.
type tapeSetupCommand struct {
	Kind string // "export", "unset", "cd", "clear", or "" if text isn't a recognized setup command.
	Env  map[string]string
	Dir  string
}

// matchTapeSetupCommand recognizes `export KEY=VALUE ...`, `unset VAR ...`,
// `cd <path>`, and `clear`. Values are split on whitespace (not
// shell-quote-aware): no real tape in this repo has a setup command whose
// value itself contains a space, so a full shell-word splitter isn't needed.
func matchTapeSetupCommand(text string) tapeSetupCommand {
	trimmed := strings.TrimSpace(text)
	switch {
	case trimmed == "clear":
		return tapeSetupCommand{Kind: "clear"}
	case strings.HasPrefix(trimmed, "cd "):
		dir := strings.Trim(strings.TrimSpace(trimmed[len("cd "):]), `"'`)
		return tapeSetupCommand{Kind: "cd", Dir: dir}
	case strings.HasPrefix(trimmed, "export "):
		return tapeSetupCommand{Kind: "export", Env: parseTapeAssignments(trimmed[len("export "):])}
	case strings.HasPrefix(trimmed, "unset "):
		env := map[string]string{}
		for _, name := range strings.Fields(trimmed[len("unset "):]) {
			env[name] = ""
		}
		return tapeSetupCommand{Kind: "unset", Env: env}
	default:
		return tapeSetupCommand{}
	}
}

// tapeResolveCd composes a `cd` target against the currently-tracked
// directory the way a real shell would: an absolute target replaces it
// outright, a relative one joins onto it. A target containing shell command
// substitution (e.g. `$(git rev-parse --show-toplevel)`, as used by
// demo/landing/hero.tape) is lifted as a literal string -- Atmos's
// working_directory: field does not evaluate `$(...)`, a known v1 limitation.
func tapeResolveCd(current, target string) string {
	if target == "" || filepath.IsAbs(target) || current == "" || strings.Contains(target, "$(") {
		return target
	}
	return filepath.Join(current, target)
}

func parseTapeAssignments(text string) map[string]string {
	env := map[string]string{}
	for _, pair := range strings.Fields(text) {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		env[key] = strings.Trim(value, `"'`)
	}
	return env
}

// tapeTrackedState carries the cwd/env state a tape's own Type "cd ..."/
// "export ..."/"unset ..." commands mutate as translation walks the
// directive list, seeded from (and never overriding) whatever the step
// explicitly configured. Mode steps snapshots this per real command child
// so each one reflects only the setup commands that preceded it; mode
// session writes the final state back onto the step once, since its single
// persistent PTY shell would reach the same end state for real.
type tapeTrackedState struct {
	explicitWorkingDirectory bool
	explicitEnvKeys          map[string]bool
	workingDirectory         string
	env                      map[string]string
}

func newTapeTrackedState(step *schema.WorkflowStep) *tapeTrackedState {
	keys := make(map[string]bool, len(step.Env))
	env := make(map[string]string, len(step.Env))
	for k, v := range step.Env {
		keys[k] = true
		env[k] = v
	}
	return &tapeTrackedState{
		explicitWorkingDirectory: step.WorkingDirectory != "",
		explicitEnvKeys:          keys,
		workingDirectory:         step.WorkingDirectory,
		env:                      env,
	}
}

func (s *tapeTrackedState) apply(setup tapeSetupCommand) {
	switch setup.Kind {
	case "export":
		for k, v := range setup.Env {
			if s.explicitEnvKeys[k] {
				continue
			}
			s.env[k] = v
		}
	case "unset":
		for k := range setup.Env {
			if s.explicitEnvKeys[k] {
				continue
			}
			delete(s.env, k)
		}
	case "cd":
		if !s.explicitWorkingDirectory {
			s.workingDirectory = tapeResolveCd(s.workingDirectory, setup.Dir)
		}
	}
}

func (s *tapeTrackedState) envSnapshot() map[string]string {
	if len(s.env) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(s.env))
	for k, v := range s.env {
		cloned[k] = v
	}
	return cloned
}

// translateTapeSteps converts a tape's directives into mode: steps children:
// each `Type "cmd" Enter` becomes a {type: simulate} (visual typing) child
// paired with a real {type: shell} (executed, exit-code-tracked) child --
// the entire reason to choose mode: steps for a tape. Setup idioms (cd,
// export, unset, clear) are recognized unconditionally (mode: steps has no
// PTY, so there is no "hidden" concept the way session mode has Hide/Show)
// and lift into tracked cwd/env state applied to every subsequent real
// shell child, instead of being replayed as independent, state-losing
// subprocess steps. Sleep/Wait are dropped (steps execute synchronously, so
// they're redundant, not incompatible). Screenshot works the same as
// session mode. Anything with no discrete-step equivalent -- bare
// keypresses, Hide/Show, a Type with no trailing Enter -- is a hard error
// naming the offending line.
func translateTapeSteps(stepName string, directives []asciicast.TapeDirective, state *tapeTrackedState) ([]schema.WorkflowStep, error) {
	var children []schema.WorkflowStep
	i := 0
	for i < len(directives) {
		d := &directives[i]
		switch d.Kind {
		case asciicast.TapeSet, asciicast.TapeOutput, asciicast.TapeRequire, asciicast.TapeSource:
			i++
		case asciicast.TapeSleep, asciicast.TapeWait:
			logTapeStepsPacingDropped(stepName, d)
			i++
		case asciicast.TapeScreenshot:
			children = append(children, schema.WorkflowStep{Type: schema.TaskTypeScreenshot, Path: d.Value})
			i++
		case asciicast.TapeType:
			next, updated, err := translateTapeStepsType(children, directives, i, state)
			if err != nil {
				return nil, err
			}
			children, i = updated, next
		case asciicast.TapeHide, asciicast.TapeShow, asciicast.TapeKey:
			return nil, tapeDirectiveError(d, fmt.Errorf(
				"%w: switch this cast step to mode: session, or remove this directive", ErrTapeDirectiveRequiresSessionMode,
			))
		case asciicast.TapeCopy, asciicast.TapePaste, asciicast.TapeEnv:
			return nil, tapeDirectiveError(d, fmt.Errorf("%w: rewrite the tape before using tape or tape_file", ErrUnsupportedTapeDirective))
		default:
			i++
		}
	}
	return children, nil
}

func logTapeStepsPacingDropped(stepName string, d *asciicast.TapeDirective) {
	log.Warn("tape: directive has no effect in mode: steps, skipping",
		"reason", "step execution is already synchronous", "kind", tapeDirectiveKindName(d.Kind),
		"step", stepName, "file", d.File, "line", d.Line)
}

// translateTapeStepsType handles one mode: steps TapeType directive at index
// i: a Type with no trailing Enter is a hard error (no discrete-step
// equivalent); a recognized setup command (cd/export/unset/clear) lifts into
// tracked state instead of producing a real step (with clear also dropping
// its own simulate narration, since there is nothing to visibly clear);
// everything else becomes the simulate+shell pairing that gives mode: steps
// its real, per-command exit codes. Returns the advanced index and the
// possibly-extended children slice.
func translateTapeStepsType(children []schema.WorkflowStep, directives []asciicast.TapeDirective, i int, state *tapeTrackedState) (int, []schema.WorkflowStep, error) {
	d := &directives[i]
	cmd := tapeConsumeTypedCommand(directives, i)
	next := i + cmd.Consumed
	if !cmd.HasEnter {
		return 0, nil, tapeDirectiveError(d, fmt.Errorf(
			"%w: a Type with no trailing Enter has no mode: steps equivalent", ErrTapeDirectiveRequiresSessionMode,
		))
	}
	children = append(children, schema.WorkflowStep{Type: schema.TaskTypeSimulate, Mode: "typed", Text: cmd.Text})
	if setup := matchTapeSetupCommand(cmd.Text); setup.Kind != "" {
		state.apply(setup)
		if setup.Kind == "clear" {
			children = children[:len(children)-1] // Drop the simulate child too: nothing to visibly clear.
		}
		return next, children, nil
	}
	children = append(children, schema.WorkflowStep{
		Type:             schema.TaskTypeShell,
		Command:          cmd.Text,
		WorkingDirectory: state.workingDirectory,
		Env:              state.envSnapshot(),
	})
	return next, children, nil
}
