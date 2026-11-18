package command_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/plugins/ssh/command"
)

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{}, {"help"}, {"--help"}, {"-h"}, {"key", "list", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out, errOut := newRig(t).run(args...)

			assert.Equal(t, command.ExitOK, code)
			assert.Contains(t, out, "Usage:")
			assert.Empty(t, errOut)
		})
	}
}

// The command fixture in plugin.tests.yaml pins the help text byte for byte, so a change to either
// one without the other fails here before it fails in "atmos plugin test".
func TestHelpMatchesThePluginFixture(t *testing.T) {
	data, err := os.ReadFile("../plugin.tests.yaml")
	require.NoError(t, err)
	var fixture struct {
		Commands []struct {
			Arguments []string `yaml:"arguments"`
			Stdout    string   `yaml:"stdout"`
		} `yaml:"commands"`
	}
	require.NoError(t, yaml.Unmarshal(data, &fixture))
	require.NotEmpty(t, fixture.Commands)

	for _, c := range fixture.Commands {
		_, out, _ := newRig(t).run(c.Arguments...)
		assert.Equal(t, c.Stdout, out)
	}
}

func TestMistakesExitWithUsageCode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown command", []string{"frobnicate"}, "unknown command"},
		{"half a command", []string{"key"}, "unknown command"},
		{"unknown flag", []string{"key", "list", "--bogus"}, "unknown flag --bogus"},
		{"short flag", []string{"key", "list", "-x"}, "unknown flag"},
		{"flag without value", []string{"key", "show", "ops", "--format"}, "needs a value"},
		{"bool flag with value", []string{"key", "delete", "ops", "--yes=true"}, "takes no value"},
		{"flag of another command", []string{"key", "list", "--yes"}, "does not apply"},
		{"missing name", []string{"key", "show"}, "usage: key show"},
		{"extra argument", []string{"agent", "status", "extra"}, "usage: agent status"},
		{"bad format", []string{"key", "show", "ops", "--format", "xml"}, "unknown format"},
		{"bad lines", []string{"agent", "log", "--lines", "zero"}, "not a positive number"},
		{"bad duration", []string{"test", "ops", "--soak", "soon"}, "not a duration"},
		{"zero interval", []string{"test", "ops", "--soak", "1m", "--interval", "0s"}, "longer than zero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.createKey("ops")

			code, out, errOut := r.run(tt.args...)

			assert.Equal(t, command.ExitUsage, code)
			assert.Empty(t, out)
			assert.Contains(t, errOut, tt.want)
		})
	}
}

func TestFlagsMayComeBeforeTheCommandAndTakeEqualsForm(t *testing.T) {
	r := newRig(t)
	r.createKey("ops")

	code, out, _ := r.run("--root="+r.paths.Root(), "key", "show", "ops", "--format=json")

	assert.Equal(t, command.ExitOK, code)
	assert.Contains(t, out, `"name": "ops"`)
}

func TestDoubleDashEndsFlags(t *testing.T) {
	r := newRig(t)

	code, _, errOut := r.run("key", "show", "--", "--format")

	assert.Equal(t, command.ExitFailure, code, "the word after -- is a key name")
	assert.Contains(t, errOut, "no such key")
}

func TestRootComesFromTheFlagThenTheSettings(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name     string
		settings map[string]any
		args     []string
		want     string
	}{
		{"neither", nil, []string{"key", "list"}, ""},
		{"settings", map[string]any{"root": dir}, []string{"key", "list"}, dir},
		{"flag", nil, []string{"key", "list", "--root", dir}, dir},
		{"flag wins", map[string]any{"root": "/ignored"}, []string{"key", "list", "--root", dir}, dir},
		{"settings of another type", map[string]any{"root": 5}, []string{"key", "list"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)

			code, _, errOut := r.runWith(tt.settings, tt.args...)

			assert.Equal(t, command.ExitOK, code, errOut)
			require.Len(t, r.roots, 1)
			assert.Equal(t, tt.want, r.roots[0])
		})
	}
}
