package step

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// castRecordingActive is the process-wide recording guard. It is held from just
// before the recorder and UI environment are installed until the restore func
// returned by startStepRecorder runs, so the full SetRecorder-to-restore
// lifetime of one recording is exclusive. It is a non-blocking guard rather
// than a blocking lock on purpose: a cast step nested inside another cast step
// (or re-entering on the same goroutine) would otherwise wait on itself forever.
// The UI environment lock is not used for this; it only protects the UI environment state.
var castRecordingActive atomic.Bool

// acquireCastRecording claims the process-wide recording guard.
// It returns a release func that is safe to call more than once.
func acquireCastRecording(name string) (func(), error) {
	if !castRecordingActive.CompareAndSwap(false, true) {
		return nil, fmt.Errorf("%w: cannot start cast step '%s'; cast steps cannot be nested or run in parallel or matrix branches", errUtils.ErrCastRecordingActive, name)
	}
	return sync.OnceFunc(func() { castRecordingActive.Store(false) }), nil
}

func startStepRecorder(step *schema.WorkflowStep, vars *Variables) (*asciicast.Recorder, func(), error) {
	applyCastRecordingDefaults(step)
	if vars == nil {
		vars = NewVariables()
	}
	path := ""
	if step.CastOutput != nil {
		path = step.CastOutput.Cast
	}
	title, err := vars.Resolve(step.Title)
	if err != nil {
		return nil, nil, err
	}
	env, err := castRecorderEnv(step, vars)
	if err != nil {
		return nil, nil, err
	}
	releaseRecording, err := acquireCastRecording(step.Name)
	if err != nil {
		return nil, nil, err
	}
	uiRestore := forceRecordingUIEnv(env)
	width, height := castRecorderDimensions(step)
	outputRate, err := parseDurationDefault(step.Rate, 0)
	if err != nil {
		uiRestore()
		releaseRecording()
		return nil, nil, err
	}
	rec, err := asciicast.Start(&asciicast.Options{
		Path:       path,
		BasePath:   viper.GetString("cast.recording.base_path"),
		Name:       step.Name,
		Title:      title,
		Command:    castCommandArgs(step),
		Width:      width,
		Height:     height,
		RecordIn:   viper.GetBool("cast.recording.input"),
		Explicit:   path != "",
		Overwrite:  true, // Declarative steps own their output path; commit replaces it.
		Env:        env,
		OutputRate: outputRate,
	})
	if err != nil {
		uiRestore()
		releaseRecording()
		return nil, nil, err
	}
	ioRestore := iolib.SetRecorder(rec)
	// The guard is released last, after the recorder and UI environment are
	// restored, so the next recording never observes this one's state. The
	// returned func is idempotent.
	return rec, sync.OnceFunc(func() {
		ioRestore()
		uiRestore()
		releaseRecording()
	}), nil
}

// castUIEnvState is the shared state behind forceRecordingUIEnv. The original
// environment is captured here, not in a per-call closure, so whichever
// recording exits last (not necessarily the first to start) performs the
// restore. It guards forceRecordingUIEnv against nested or parallel-sibling
// `type: cast` steps (a cast step can be one branch of a top-level workflow
// `type: parallel`/`matrix` step): only the first call mutates the process env
// and re-detected UI state, and only the last exit restores it, so overlapping
// recordings never clobber or prematurely restore each other.
type castUIEnvState struct {
	mu    sync.Mutex
	depth int
	// prior maps each forced key to its original value (nil when it was unset).
	prior map[string]*string
}

var castUIEnv castUIEnvState

// castUIEnvKeys are the environment variables that influence color/TTY
// detection; only these are captured/restored around a recording.
var castUIEnvKeys = []string{"NO_COLOR", "ATMOS_FORCE_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE", "COLORTERM", "ATMOS_FORCE_TTY"}

// forceRecordingUIEnv makes the current process's themed UI output (step
// headers/footers, toasts, spinners — anything rendered via pkg/ui during the
// recording) match the color/TTY-forcing environment configured for a
// `type: cast` step's recording.
//
// Env vars configured on a cast step (e.g. ATMOS_FORCE_COLOR/ATMOS_FORCE_TTY
// from a `.env.recording` block) are otherwise only ever applied to the child
// subprocess environments of nested `type: shell` steps — pkg/ui's terminal
// detection, performed once at process startup from the real OS environment,
// never sees them. That leaves anything rendered in-process during the
// recording (step labels, toast/spinner messages) unstyled — or the spinner
// forced to its non-interactive fallback — regardless of how the recording's
// env is configured. This applies the same env vars to the current process
// and calls ui.ReinitFormatter to re-run that same detection, so in-process
// output captured into the recording renders identically to the recorded
// subprocess output.
//
// Returns a restore func that reverts both the environment and the
// re-detected UI state; always call it, even on error paths.
func forceRecordingUIEnv(env map[string]string) func() {
	relevant := make(map[string]string, len(castUIEnvKeys))
	for _, key := range castUIEnvKeys {
		if value, ok := env[key]; ok {
			relevant[key] = value
		}
	}
	if len(relevant) == 0 {
		return func() {}
	}

	castUIEnv.mu.Lock()
	defer castUIEnv.mu.Unlock()
	castUIEnv.depth++
	if castUIEnv.prior == nil {
		castUIEnv.prior = make(map[string]*string, len(relevant))
	}
	// The first scope always re-detects; later scopes only when they change the env.
	changed := castUIEnv.depth == 1
	for key, value := range relevant {
		old, wasSet := os.LookupEnv(key)
		if _, captured := castUIEnv.prior[key]; !captured {
			if wasSet {
				oldCopy := old
				castUIEnv.prior[key] = &oldCopy
			} else {
				castUIEnv.prior[key] = nil
			}
		}
		if !wasSet || old != value {
			changed = true
		}
		_ = os.Setenv(key, value)
	}
	if changed {
		ui.ReinitFormatter()
	}

	// Every call shares this one cleanup path; sync.OnceFunc keeps a repeated
	// restore call from decrementing the depth twice.
	return sync.OnceFunc(releaseCastUIEnv)
}

// releaseCastUIEnv drops one forceRecordingUIEnv scope. When the last scope
// exits it restores the original environment captured in the shared state and
// re-detects the UI formatter.
func releaseCastUIEnv() {
	castUIEnv.mu.Lock()
	defer castUIEnv.mu.Unlock()
	castUIEnv.depth--
	if castUIEnv.depth > 0 {
		return
	}
	for key, old := range castUIEnv.prior {
		if old == nil {
			_ = os.Unsetenv(key)
		} else {
			_ = os.Setenv(key, *old)
		}
	}
	castUIEnv.prior = nil
	ui.ReinitFormatter()
}

func castRecorderEnv(step *schema.WorkflowStep, vars *Variables) (map[string]string, error) {
	env := make(map[string]string)
	for key, value := range vars.Env {
		env[key] = value
	}
	if len(step.Env) == 0 {
		return env, nil
	}
	resolvedEnv, err := vars.ResolveStepEnvMap(step, step.Env)
	if err != nil {
		return nil, fmt.Errorf("step '%s': %w", step.Name, err)
	}
	for key, value := range resolvedEnv {
		env[key] = value
	}
	return env, nil
}

func castRecorderDimensions(step *schema.WorkflowStep) (int, int) {
	width := step.Width
	if width <= 0 {
		width = viper.GetInt("cast.recording.width")
	}
	height := step.Height
	if height <= 0 {
		height = viper.GetInt("cast.recording.height")
	}
	return width, height
}

func applyCastRecordingDefaults(step *schema.WorkflowStep) {
	if step.Defaults == nil || step.Defaults.Cast == nil {
		return
	}
	defaults := step.Defaults.Cast
	if step.Rate == "" {
		step.Rate = defaults.Rate
	}
	if step.Width <= 0 {
		step.Width = defaults.Width
	}
	if step.Height <= 0 {
		step.Height = defaults.Height
	}
}
