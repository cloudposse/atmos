package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorOptionsFromArgs(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		noColorEnv string
		logsEnv    string
		want       ColorOptions
	}{
		{"bare flag before command", []string{"--logs-level=debug", "--no-color", "version"}, "", "", ColorOptions{NoColor: true, NoColorSet: true}},
		{"help before opt-out", []string{"--help", "--no-color"}, "", "", ColorOptions{NoColor: true, NoColorSet: true}},
		{"explicit false", []string{"--no-color=false", "version"}, "", "", ColorOptions{NoColorSet: true}},
		{"last boolean wins", []string{"--no-color", "--no-color=false"}, "", "", ColorOptions{NoColorSet: true}},
		{"separator", []string{"version", "--", "--no-color", "--logs-color=true"}, "", "", ColorOptions{}},
		{"NO_COLOR is authoritative", []string{"--no-color=false"}, "0", "true", ColorOptions{NoColor: true, NoColorSet: true, LogsColor: "true"}},
		{"logging CLI beats env", []string{"--logs-color=false", "version"}, "", "true", ColorOptions{LogsColor: "false"}},
		{"logging bare boolean", []string{"--logs-color", "version"}, "", "false", ColorOptions{LogsColor: "true"}},
		{"environment boolean normalized", []string{"version"}, "", "0", ColorOptions{LogsColor: "false"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", tt.noColorEnv)
			t.Setenv("ATMOS_NO_COLOR", "")
			t.Setenv("ATMOS_LOGS_COLOR", tt.logsEnv)
			assert.Equal(t, tt.want, ColorOptionsFromArgs(tt.args))
		})
	}
}
