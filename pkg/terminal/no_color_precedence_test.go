package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoColorOverridesEveryColorSource(t *testing.T) {
	for _, tty := range []bool{false, true} {
		for _, force := range []bool{false, true} {
			cfg := &Config{NoColor: true, ForceColor: force, EnvCLIColorForce: true, EnvCI: true, Color: true}
			assert.False(t, cfg.ShouldUseColor(tty))
			assert.Equal(t, ColorNone, cfg.DetectColorProfile(tty))
			assert.Equal(t, ColorNone, New(WithConfig(cfg)).ColorProfile())
		}
	}
}
