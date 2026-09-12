package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Config.CLI() must fall back to ComponentType when CLIName is unset — the
// common case for types whose CLI invocation matches their internal
// ComponentType exactly (terraform, helmfile, packer).
func TestConfig_CLI_FallsBackToComponentType(t *testing.T) {
	c := &Config{ComponentType: "terraform"}
	assert.Equal(t, "terraform", c.CLI())
}

// Config.CLI() must return CLIName verbatim when set — used by types whose
// CLI command path differs from their internal ComponentType (e.g.
// aws/cloudformation is invoked as "aws cloudformation").
func TestConfig_CLI_UsesCLINameWhenSet(t *testing.T) {
	c := &Config{ComponentType: "aws/cloudformation", CLIName: "aws cloudformation"}
	assert.Equal(t, "aws cloudformation", c.CLI())
}
