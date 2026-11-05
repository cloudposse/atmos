package reexec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArgsDefaultToOSArgs(t *testing.T) {
	assert.Equal(t, os.Args, Args())
	assert.Zero(t, ScriptArgs())
}

func TestSetOriginalArgsRecordsAndRestores(t *testing.T) {
	original := []string{"atmos", "--chdir=d", "./tool", "hello"}
	restore := SetOriginalArgs(original, 2)
	assert.Equal(t, original, Args())
	assert.Equal(t, 2, ScriptArgs())

	original[0] = "mutated"
	assert.Equal(t, "atmos", Args()[0], "the recorded value is a copy")
	Args()[0] = "mutated"
	assert.Equal(t, "atmos", Args()[0], "callers get a copy")

	nested := SetOriginalArgs([]string{"inner"}, 0)
	assert.Equal(t, []string{"inner"}, Args())
	nested()
	assert.Equal(t, []string{"atmos", "--chdir=d", "./tool", "hello"}, Args())

	restore()
	assert.Equal(t, os.Args, Args())
	assert.Zero(t, ScriptArgs())
}

func TestStripBeforeScriptLeavesScriptArgumentsUntouched(t *testing.T) {
	args := []string{"atmos", "--chdir=d", "-C", "x", "./tool", "--chdir=mine", "-Cmine", "--use-version", "script"}
	assert.Equal(t,
		[]string{"atmos", "./tool", "--chdir=mine", "-Cmine", "--use-version", "script"},
		StripBeforeScript(args, 5, StripChdirArgs),
		"only the arguments before the script are stripped")
	assert.Equal(t, []string{"atmos"}, StripBeforeScript(args, 0, func([]string) []string { return []string{"atmos"} }), "no script: strip everything")
	assert.Equal(t, []string{"atmos"}, StripBeforeScript(args, 99, func([]string) []string { return []string{"atmos"} }), "an impossible count means no script")
}
