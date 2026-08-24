package fps

import (
	"os"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/stretchr/testify/assert"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name string
		set  bool
		val  string
		want int
	}{
		{"unset keeps default", false, "", 0},
		{"empty keeps default", true, "", 0},
		{"valid value", true, "4", 4},
		{"one fps", true, "1", 1},
		{"zero ignored", true, "0", 0},
		{"negative ignored", true, "-5", 0},
		{"non-numeric ignored", true, "fast", 0},
		{"above max clamped", true, "1000", MaxFPS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(EnvVar, tt.val)
			} else {
				t.Setenv(EnvVar, "")
				_ = os.Unsetenv(EnvVar)
			}
			assert.Equal(t, tt.want, FromEnv())
		})
	}
}

func TestApply(t *testing.T) {
	t.Run("override applied when set", func(t *testing.T) {
		t.Setenv(EnvVar, "4")
		s := spinner.New()
		s.Spinner = spinner.Dot
		Apply(&s)
		assert.Equal(t, time.Second/4, s.Spinner.FPS)
	})

	t.Run("default preserved when unset", func(t *testing.T) {
		t.Setenv(EnvVar, "")
		_ = os.Unsetenv(EnvVar)
		s := spinner.New()
		s.Spinner = spinner.Dot
		before := s.Spinner.FPS
		Apply(&s)
		assert.Equal(t, before, s.Spinner.FPS)
	})

	t.Run("nil is a no-op", func(t *testing.T) {
		t.Setenv(EnvVar, "4")
		assert.NotPanics(t, func() { Apply(nil) })
	})
}
