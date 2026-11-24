package dependencies

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// The same dependencies must always produce the same PATH. Tools can provide executables
// with the same name, so directory order decides which one runs and must not depend on map
// iteration order.
func TestToolchainPathOrderIsDeterministic(t *testing.T) {
	config, _ := defaultsFixture(t, "")
	config.Toolchain.Install = schema.ToolchainInstallNever
	deps := map[string]string{}
	var expected []string
	for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		config.Toolchain.Aliases[name] = "acme/" + name
		expected = append(expected, filepath.Dir(installedDefaultBinary(t, config, "acme", name, "1.0.0")))
		deps[name] = "1.0.0"
	}

	for range 50 {
		env, err := NewEnvironmentFromDeps(config, deps)
		require.NoError(t, err)
		require.Equal(t, expected, env.ToolchainDirs(), "directories follow tool name order")
		require.True(t, strings.HasPrefix(env.PATH(), strings.Join(expected, string(filepath.ListSeparator))), "PATH must list them in the same order: %s", env.PATH())
	}
}
