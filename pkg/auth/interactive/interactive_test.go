package interactive

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

func newEnv(t *testing.T, stdin, stderr, enabled, ci bool) Environment {
	t.Helper()
	ctrl := gomock.NewController(t)
	tty := term.NewMockTTYDetector(ctrl)
	tty.EXPECT().IsTTYForStdin().Return(stdin).AnyTimes()
	tty.EXPECT().IsTTYForStderr().Return(stderr).AnyTimes()
	return Environment{
		TTY:     tty,
		Enabled: func() bool { return enabled },
		CI:      func() bool { return ci },
	}
}

func TestEnvironment_Available(t *testing.T) {
	tests := []struct {
		name    string
		stdin   bool
		stderr  bool
		enabled bool
		ci      bool
		want    bool
	}{
		{name: "stdin and stderr terminals", stdin: true, stderr: true, enabled: true, want: true},
		// Regression: huh draws on stderr, so a captured stderr makes the prompt invisible.
		{name: "stdin terminal, stderr captured", stdin: true, stderr: false, enabled: true, want: false},
		{name: "stdin piped, stderr terminal", stdin: false, stderr: true, enabled: true, want: false},
		{name: "neither is a terminal", stdin: false, stderr: false, enabled: true, want: false},
		{name: "interactive disabled", stdin: true, stderr: true, enabled: false, want: false},
		{name: "CI environment", stdin: true, stderr: true, enabled: true, ci: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, newEnv(t, tt.stdin, tt.stderr, tt.enabled, tt.ci).Available())
		})
	}
}

func TestEnvironment_Available_NilDependenciesFailClosed(t *testing.T) {
	assert.False(t, Environment{}.Available(), "no TTY detector means no prompt")
}

func TestAvailable_FollowsInteractiveFlag(t *testing.T) {
	original := viper.GetBool(viperKeyInteractive)
	t.Cleanup(func() { viper.Set(viperKeyInteractive, original) })

	viper.Set(viperKeyInteractive, false)
	assert.False(t, Available(), "--interactive=false must disable prompts")
	assert.False(t, Default().Enabled())
}
