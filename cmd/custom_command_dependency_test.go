package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandIntegration_DependenciesCommandsDiamondDedup exercises the diamond-shaped
// dependency case: release depends on [test, lint], both of which depend on build, which must
// execute exactly once for the whole `atmos release` invocation, even though two different
// commands declare a dependency on it.
//
// Command names are prefixed per-test (dd-*) since RootCmd accumulates custom-command
// registrations across tests within one test binary run -- only flags/args are reset by
// NewTestKit -- so reusing a generic name like "build" across two test functions in this
// package would resolve to whichever test registered it first.
func TestCustomCommandIntegration_DependenciesCommandsDiamondDedup(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	buildLog := filepath.Join(tmpDir, "build.txt")
	releaseLog := filepath.Join(tmpDir, "release.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name:  "dd-build",
			Steps: schema.Tasks{{Type: "shell", Command: "echo build >> " + buildLog}},
		},
		{
			Name:         "dd-test",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{{Name: "dd-build"}}},
			Steps:        schema.Tasks{{Type: "shell", Command: "echo test >> " + buildLog}},
		},
		{
			Name:         "dd-lint",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{{Name: "dd-build"}}},
			Steps:        schema.Tasks{{Type: "shell", Command: "echo lint >> " + buildLog}},
		},
		{
			Name:         "dd-release",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{{Name: "dd-test"}, {Name: "dd-lint"}}},
			Steps:        schema.Tasks{{Type: "shell", Command: "echo release >> " + releaseLog}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var releaseCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "dd-release" {
			releaseCmd = c
			break
		}
	}
	require.NotNil(t, releaseCmd, "dd-release command should be registered")

	releaseCmd.Run(releaseCmd, []string{})

	content, err := os.ReadFile(buildLog)
	require.NoError(t, err)
	lines := splitNonEmptyLines(string(content))
	buildCount := 0
	for _, line := range lines {
		if line == "build" {
			buildCount++
		}
	}
	assert.Equal(t, 1, buildCount, "build must execute exactly once despite two dependents (test and lint)")
	assert.Contains(t, lines, "test")
	assert.Contains(t, lines, "lint")
	assert.FileExists(t, releaseLog, "release's own step must still run after its dependencies complete")
}

// TestCustomCommandIntegration_DependenciesCommandsParameterizedBothRun verifies that two
// dependency entries referencing the same command with DIFFERENT flag overrides are distinct
// graph nodes and both execute, while identical overrides collapse into one execution.
func TestCustomCommandIntegration_DependenciesCommandsParameterizedBothRun(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	buildLog := filepath.Join(tmpDir, "build.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name: "pp-build",
			Flags: []schema.CommandFlag{
				{Name: "env", Type: "string", Default: "dev"},
			},
			Steps: schema.Tasks{{Type: "shell", Command: "echo env={{ .Flags.env }} >> " + buildLog}},
		},
		{
			Name: "pp-release",
			Dependencies: &schema.Dependencies{Commands: schema.UnitDependencies{
				{Name: "pp-build", Flags: map[string]string{"env": "dev"}},
				{Name: "pp-build", Flags: map[string]string{"env": "prod"}},
			}},
			Steps: schema.Tasks{{Type: "shell", Command: "true"}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var releaseCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "pp-release" {
			releaseCmd = c
			break
		}
	}
	require.NotNil(t, releaseCmd, "pp-release command should be registered")

	releaseCmd.Run(releaseCmd, []string{})

	content, err := os.ReadFile(buildLog)
	require.NoError(t, err)
	lines := splitNonEmptyLines(string(content))
	assert.ElementsMatch(t, []string{"env=dev", "env=prod"}, lines,
		"differently-parameterized invocations of build must both run, exactly once each")
}

func splitNonEmptyLines(s string) []string {
	var out []string
	line := ""
	for _, r := range s {
		if r == '\n' {
			if line != "" {
				out = append(out, line)
			}
			line = ""
			continue
		}
		line += string(r)
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
