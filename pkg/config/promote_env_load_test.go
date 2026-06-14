package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// writeProfileFixture creates a minimal profile under <dir>/profiles/<name> whose
// atmos.yaml sets a distinctive stacks.name_template so the test can confirm the
// profile was actually merged.
func writeProfileFixture(t *testing.T, dir, name, nameTemplate string) {
	t.Helper()
	profileDir := filepath.Join(dir, "profiles", name)
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(profileDir, AtmosConfigFileName),
		[]byte("stacks:\n  name_template: "+nameTemplate+"\n"),
		0o644,
	))
}

// TestLoadConfig_PinsAtmosProfileFromEnvSection verifies that ATMOS_PROFILE declared in
// a project `.env` (included via `env: !include .env`) is promoted into the process
// environment and honored by Atmos's own profile resolution within the same load.
func TestLoadConfig_PinsAtmosProfileFromEnvSection(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"),
		[]byte("ATMOS_PROFILE=pinned\nAWS_REGION=us-west-2\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName),
		[]byte("base_path: ./\nenv: !include .env\n"), 0o644))
	writeProfileFixture(t, tmpDir, "pinned", "pinned-template")

	t.Chdir(tmpDir)
	// Isolate ATMOS_PROFILE: register it for restore, then ensure it is truly unset so
	// the project `.env` value is what gets promoted.
	t.Setenv("ATMOS_PROFILE", "")
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	// The ATMOS_PROFILE from `.env` was promoted into the process environment.
	assert.Equal(t, "pinned", os.Getenv("ATMOS_PROFILE"))
	// The pinned profile was actually resolved and merged.
	assert.Equal(t, "pinned-template", atmosConfig.Stacks.NameTemplate)
	// A non-ATMOS_ key from the `.env` must NOT be promoted into the Atmos process env
	// (it still flows to subprocesses via MergeGlobalEnv, but not here).
	_, awsSet := os.LookupEnv("AWS_REGION")
	assert.False(t, awsSet, "non-ATMOS_ keys must not be promoted into the Atmos process env")
}

// TestLoadConfig_RealEnvWinsOverEnvSection verifies that an exported ATMOS_PROFILE takes
// precedence over the value in the project `.env`.
func TestLoadConfig_RealEnvWinsOverEnvSection(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"),
		[]byte("ATMOS_PROFILE=pinned\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName),
		[]byte("base_path: ./\nenv: !include .env\n"), 0o644))
	writeProfileFixture(t, tmpDir, "pinned", "pinned-template")
	writeProfileFixture(t, tmpDir, "from-shell", "shell-template")

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_PROFILE", "from-shell")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	// The exported env var is preserved (the `.env` value never overwrote it).
	assert.Equal(t, "from-shell", os.Getenv("ATMOS_PROFILE"))
	// And the shell-selected profile was merged, not the one named in `.env`.
	assert.Equal(t, "shell-template", atmosConfig.Stacks.NameTemplate)
}

// TestLoadConfig_NoAtmosProfileInEnvSection verifies the negative path: when the `.env`
// has no ATMOS_PROFILE, nothing is promoted and profile resolution is unaffected.
func TestLoadConfig_NoAtmosProfileInEnvSection(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, ".env"),
		[]byte("AWS_REGION=us-west-2\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName),
		[]byte("base_path: ./\nenv: !include .env\n"), 0o644))

	t.Chdir(tmpDir)
	t.Setenv("ATMOS_PROFILE", "")
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))

	_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	// No ATMOS_PROFILE was promoted (there was none to promote), so resolution is a no-op.
	_, set := os.LookupEnv("ATMOS_PROFILE")
	assert.False(t, set, "ATMOS_PROFILE must remain unset when the .env does not define it")
}
