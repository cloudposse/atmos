//go:build !windows

package utils

import (
	"bytes"
	"os"
	"testing"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStyledTextRespectsCLICOLOR(t *testing.T) {
	master, slave, err := pty.Open()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = slave
	t.Cleanup(func() { os.Stdout = original; _ = slave.Close(); _ = master.Close() })
	for _, key := range []string{"NO_COLOR", "ATMOS_NO_COLOR", "ATMOS_FORCE_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "CI"} {
		t.Setenv(key, "")
	}
	t.Setenv("TERM", "xterm-256color")
	for _, tt := range []struct {
		name, cliColor, noColor, force string
		wantColor                      bool
	}{
		{"terminal color prerequisite", "1", "", "", true},
		{"zero disables terminal logo", "0", "", "", false},
		{"NO_COLOR beats CLICOLOR", "1", "1", "", false},
		{"force overrides zero", "0", "", "1", true},
		{"NO_COLOR beats both", "1", "1", "1", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLICOLOR", tt.cliColor)
			t.Setenv("NO_COLOR", tt.noColor)
			t.Setenv("CLICOLOR_FORCE", tt.force)
			var output bytes.Buffer
			require.NoError(t, PrintStyledTextToSpecifiedOutput(&output, "A"))
			assert.Equal(t, tt.wantColor, bytes.Contains(output.Bytes(), []byte("\x1b")))
		})
	}
}
