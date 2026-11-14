package step

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

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
	uiRestore := forceRecordingUIEnv(env)
	width, height := castRecorderDimensions(step)
	outputRate, err := parseDurationDefault(step.Rate, 0)
	if err != nil {
		uiRestore()
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
		return nil, nil, err
	}
	ioRestore := iolib.SetRecorder(rec)
	return rec, func() {
		ioRestore()
		uiRestore()
	}, nil
}

// castUIEnvMu and castUIEnvDepth guard forceRecordingUIEnv against nested or
// parallel-sibling `type: cast` steps (a cast step can be one branch of a
// top-level workflow `type: parallel`/`matrix` step): only the outermost call
// mutates or restores the process env and re-detected UI state, so
// overlapping recordings never clobber or prematurely restore each other.
var (
	castUIEnvMu    sync.Mutex
	castUIEnvDepth int
)

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

	castUIEnvMu.Lock()
	defer castUIEnvMu.Unlock()
	castUIEnvDepth++
	if castUIEnvDepth > 1 {
		// An outer (or concurrent sibling) cast recording already forced the
		// environment; just track this scope's exit without mutating/restoring.
		return func() {
			castUIEnvMu.Lock()
			defer castUIEnvMu.Unlock()
			castUIEnvDepth--
		}
	}

	priorEnv := make(map[string]*string, len(relevant))
	for key, value := range relevant {
		if old, ok := os.LookupEnv(key); ok {
			oldCopy := old
			priorEnv[key] = &oldCopy
		} else {
			priorEnv[key] = nil
		}
		_ = os.Setenv(key, value)
	}
	ui.ReinitFormatter()

	return func() {
		castUIEnvMu.Lock()
		defer castUIEnvMu.Unlock()
		castUIEnvDepth--
		if castUIEnvDepth > 0 {
			return
		}
		for key, old := range priorEnv {
			if old == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *old)
			}
		}
		ui.ReinitFormatter()
	}
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
