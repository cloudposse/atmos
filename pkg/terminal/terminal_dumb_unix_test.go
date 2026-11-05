//go:build !windows

package terminal

import (
	"os"
	"testing"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xterm "golang.org/x/term"
)

// TestDumbTerminal verifies that a physical TTY still uses static output when
// TERM=dumb, while explicit terminal and color overrides remain independent.
func TestDumbTerminal(t *testing.T) {
	defer setupTest(t)()
	master, slave, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = master.Close()
		_ = slave.Close()
	})
	require.True(t, xterm.IsTerminal(int(slave.Fd())))
	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = slave, slave, slave
	t.Cleanup(func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr })

	for _, tc := range []struct {
		name      string
		config    Config
		wantTTY   bool
		wantColor ColorProfile
	}{
		{name: "normal terminal", config: Config{EnvTerm: "xterm-256color"}, wantTTY: true, wantColor: Color256},
		{name: "dumb terminal", config: Config{EnvTerm: "dumb"}, wantColor: ColorNone},
		{name: "CI and color hints", config: Config{EnvTerm: "dumb", EnvCI: true, EnvColorTerm: "truecolor", Color: true}, wantColor: ColorNone},
		{name: "forced terminal only", config: Config{EnvTerm: "dumb", ForceTTY: true}, wantTTY: true, wantColor: ColorNone},
		{name: "forced color only", config: Config{EnvTerm: "dumb", ForceColor: true}, wantColor: ColorTrue},
		{name: "forced CLI color only", config: Config{EnvTerm: "dumb", EnvCLIColorForce: true}, wantColor: ColorTrue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terminal := New(WithConfig(&tc.config))
			for _, stream := range []Stream{Stdin, Stdout, Stderr} {
				assert.Equal(t, tc.wantTTY, terminal.IsTTY(stream))
			}
			assert.Equal(t, tc.wantColor, terminal.ColorProfile())
		})
	}
}
