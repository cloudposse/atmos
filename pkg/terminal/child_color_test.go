package terminal

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChildColorPreservesEffectivePreferences(t *testing.T) {
	defer setupTest(t)()
	for _, tc := range []struct {
		name         string
		config       Config
		env          []string
		tty, visible bool
		want         []string
	}{
		{"terminal forwarding", Config{}, []string{"CUSTOM=value"}, true, true, []string{"CUSTOM=value", "ATMOS_FORCE_COLOR=true"}},
		{"redirected", Config{}, nil, false, true, nil},
		{"CI does not force redirected output", Config{EnvCI: true}, nil, false, true, nil},
		{"CI terminal forwarding", Config{EnvCI: true}, nil, true, true, []string{"ATMOS_FORCE_COLOR=true"}},
		{"capture only", Config{}, nil, true, false, nil},
		{"explicit force on redirected stream", Config{ForceColor: true}, nil, false, true, []string{"ATMOS_FORCE_COLOR=true"}},
		{"parent opt out wins over force", Config{NoColor: true, ForceColor: true}, []string{"ATMOS_FORCE_COLOR=true"}, true, true, []string{"ATMOS_FORCE_COLOR=true", "NO_COLOR=1"}},
		{"parent environment opt out", Config{EnvNoColor: true}, nil, true, true, []string{"NO_COLOR=1"}},
		{"child opt out", Config{}, []string{"NO_COLOR=1"}, true, true, []string{"NO_COLOR=1"}},
		{"child clicolor opt out", Config{}, []string{"CLICOLOR=0"}, true, true, []string{"CLICOLOR=0"}},
		{"child force disabled", Config{}, []string{"ATMOS_FORCE_COLOR=false"}, true, true, []string{"ATMOS_FORCE_COLOR=false"}},
		{"child force preserved", Config{}, []string{"FORCE_COLOR=1"}, false, true, []string{"FORCE_COLOR=1"}},
		{"ordinary color respects redirection", Config{Color: true}, nil, false, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]string(nil), tc.env...)
			got := childColorEnv(&tc.config, tc.env, tc.tty, tc.visible)
			require.Equal(t, tc.want, got)
			require.Equal(t, before, tc.env, "caller environment must be preserved")
			for _, entry := range got {
				require.NotContains(t, entry, "FORCE_TTY")
			}
		})
	}
}
