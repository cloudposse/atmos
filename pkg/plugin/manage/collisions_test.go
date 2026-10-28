package manage

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
)

func lockWithExports(exports map[string][]pluginlock.Export) *pluginlock.File {
	file := &pluginlock.File{}
	file.Spec.Packages = map[string]pluginlock.Entry{}
	for alias, list := range exports {
		file.Spec.Packages[alias] = pluginlock.Entry{Exports: list}
	}
	return file
}

func TestCollisionsNameEveryOwnerInDeterministicOrder(t *testing.T) {
	shared := []pluginlock.Export{{Capability: "command", Name: "deploy"}}
	file := lockWithExports(map[string][]pluginlock.Export{"zulu": shared, "alpha": shared, "mike": shared})
	first := checkCollisions(file)
	require.ErrorIs(t, first, errUtils.ErrDuplicatePluginExtension)
	for range 50 {
		// Go randomizes map iteration; the message must not depend on it.
		require.EqualError(t, checkCollisions(file), first.Error())
	}
	assert.Contains(t, first.Error(), "command:deploy is exported by alpha, mike, and zulu")
	assert.Contains(t, errUtils.AllHints(first)[0], "Remove")
}

func TestCollisionsReportTwoOwnersAndBuiltinsDeterministically(t *testing.T) {
	file := lockWithExports(map[string][]pluginlock.Export{
		"b": {{Capability: "command", Name: "x"}}, "a": {{Capability: "command", Name: "x"}},
	})
	require.ErrorContains(t, checkCollisions(file), "command:x is exported by a and b")

	builtin := lockWithExports(map[string][]pluginlock.Export{
		"b": {{Capability: "auth.identity", Name: "aws/user"}},
	})
	err := checkCollisions(builtin)
	require.ErrorIs(t, err, errUtils.ErrDuplicatePluginExtension)
	assert.Contains(t, err.Error(), "auth.identity:aws/user conflicts with a built-in")
	assert.Contains(t, err.Error(), "b", fmt.Sprint(err))
	require.NoError(t, checkCollisions(lockWithExports(map[string][]pluginlock.Export{
		"a": {{Capability: "command", Name: "one"}}, "b": {{Capability: "command", Name: "two"}},
	})))
}
