// Package fps centralizes the optional spinner redraw-rate override so every
// spinner in Atmos honors a single control.
//
// Spinners default to the bubbles rate. When recording VHS demos, frequent
// redraws of off-camera spinners can scroll enough to trip VHS's scrollback
// handling (charmbracelet/vhs#657/#659) and hang the recording. Setting
// ATMOS_SPINNER_FPS to a lower positive integer slows every spinner; leaving it
// unset preserves the default for normal interactive use.
package fps

import (
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/spf13/viper"
)

// EnvVar is the environment variable that overrides the spinner redraw rate
// (frames per second).
const EnvVar = "ATMOS_SPINNER_FPS"

// MaxFPS bounds the override so a bad value can't busy-loop the renderer.
const MaxFPS = 60

// Apply slows a spinner model's redraw rate to ATMOS_SPINNER_FPS when that env
// var is set to a positive integer (capped at MaxFPS). When the var is unset or
// invalid the model is left at its existing rate. Call it right after creating
// (and configuring) any bubbles spinner.Model.
func Apply(s *spinner.Model) {
	if s == nil {
		return
	}
	if fps := FromEnv(); fps > 0 {
		s.Spinner.FPS = time.Second / time.Duration(fps)
	}
}

// FromEnv returns the spinner redraw rate (frames per second) parsed from
// ATMOS_SPINNER_FPS, or 0 when unset/invalid (meaning "keep the default rate").
func FromEnv() int {
	_ = viper.BindEnv(EnvVar, EnvVar)
	raw := viper.GetString(EnvVar)
	if raw == "" {
		return 0
	}
	fps, err := strconv.Atoi(raw)
	if err != nil || fps <= 0 {
		return 0
	}
	if fps > MaxFPS {
		return MaxFPS
	}
	return fps
}
