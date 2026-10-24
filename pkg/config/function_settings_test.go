package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecConcurrencyConfiguration checks the default, explicit limits, and invalid configuration.
func TestExecConcurrencyConfiguration(t *testing.T) {
	for _, tc := range []struct {
		value   string
		want    int
		invalid bool
	}{
		{"", 1, false}, {"1", 1, false}, {"4", 4, false}, {"0", 0, true}, {"-1", 0, true},
	} {
		t.Run("limit="+tc.value, func(t *testing.T) {
			content := "base_path: .\n"
			if tc.value != "" {
				content += fmt.Sprintf("settings:\n  functions:\n    exec:\n      max_concurrency: %s\n", tc.value)
			}
			writeEditionTestConfig(t, content)
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			if tc.invalid {
				require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, config.Settings.Functions.Exec.Concurrency())
		})
	}
}
