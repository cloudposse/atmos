package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Config.CLI falls back to ComponentType for matching internal/CLI names,
// while returning an explicit CLIName verbatim when the command path differs.
func TestConfig_CLI(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		config Config
		want   string
	}{
		{name: "falls back to component type", config: Config{ComponentType: "terraform"}, want: "terraform"},
		{name: "uses CLI name when set", config: Config{ComponentType: "aws/cloudformation", CLIName: "aws cloudformation"}, want: "aws cloudformation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.config.CLI())
		})
	}
}
