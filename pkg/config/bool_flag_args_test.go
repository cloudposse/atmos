package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFlagsFromArgs_BoolFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want map[string]string
	}{
		{"verbose bare before command", []string{"atmos", "--verbose", "version"}, map[string]string{"verbose": "true"}},
		{"verbose false literal", []string{"atmos", "--verbose", "false", "version"}, map[string]string{"verbose": "false"}},
		{"verbose upper TRUE literal", []string{"atmos", "--verbose", "TRUE", "version"}, map[string]string{"verbose": "true"}},
		{"verbose equals", []string{"atmos", "--verbose=false", "version"}, map[string]string{"verbose": "false"}},
		{"logs-color false", []string{"atmos", "--logs-color", "false", "version"}, map[string]string{"logs-color": "false"}},
		{"no-color bare", []string{"atmos", "--no-color", "version"}, map[string]string{"no-color": "true"}},
		{"no-color 1 is not a value", []string{"atmos", "--no-color", "1"}, map[string]string{"no-color": "true"}},
		{"string flag still consumes value", []string{"atmos", "--logs-level", "Debug", "--verbose", "false"}, map[string]string{"logs-level": "Debug", "verbose": "false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseFlagsFromArgs(tt.args))
		})
	}
}
