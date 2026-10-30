package terraform

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// When the process PATH already starts with the toolchain directories (for example an Atmos run
// nested inside a toolchain shell), there is nothing left to prepend: the selected PATH is used as is.
func TestToolchainPathEnvWithToolchainDirectoriesAlreadyInAmbientPath(t *testing.T) {
	harness := newToolchainHarness(t, "1.0.0")
	info := harness.info(t, &toolchainScenario{identity: "development", tofuVersion: "1.0.0", toolVersion: "1.0.0"}, filepath.Join(t.TempDir(), "probe.jsonl"))
	tenv, err := resolveAndInstallToolchainDeps(harness.config, info)
	require.NoError(t, err)
	full := tenv.PATH()
	require.NotEmpty(t, full)
	t.Setenv("PATH", full)

	selected := filepath.Join(t.TempDir(), "selected")
	got := toolchainPathEnv(&schema.ConfigAndStacksInfo{ComponentEnvList: []string{"PATH=" + selected}}, tenv)
	require.Equal(t, []string{"PATH=" + selected}, got)

	// An empty selected PATH is the same case with nothing to extend: only the toolchain prefix remains, which is empty.
	got = toolchainPathEnv(&schema.ConfigAndStacksInfo{ComponentEnvList: []string{"PATH="}}, tenv)
	require.Equal(t, []string{"PATH="}, got)
	require.False(t, strings.Contains(got[0], full), "the ambient toolchain directories must not be re-added")
}
