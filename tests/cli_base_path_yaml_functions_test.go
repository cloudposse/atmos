package tests

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

func TestBasePathWithYAMLFunctions(t *testing.T) {
	// Initialize atmosRunner if needed
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Get absolute path to fixtures
	fixturesDir := filepath.Join("fixtures", "scenarios", "basic")
	absFixturesDir, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	t.Run("--base-path with !repo-root function", func(t *testing.T) {
		// Set TEST_GIT_ROOT to simulate git root
		t.Setenv("TEST_GIT_ROOT", absFixturesDir)

		cmd := atmosRunner.Command("--base-path=!repo-root", "terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed - !repo-root should resolve to absFixturesDir
		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("--base-path with literal path", func(t *testing.T) {
		// Test that literal paths still work (no breaking change)
		cmd := atmosRunner.Command("--base-path="+absFixturesDir, "terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("--base-path with !env function", func(t *testing.T) {
		// Set environment variable for !env function
		t.Setenv("TEST_INFRA_PATH", absFixturesDir)

		cmd := atmosRunner.Command("--base-path=!env TEST_INFRA_PATH", "terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("ATMOS_BASE_PATH env var with !repo-root function", func(t *testing.T) {
		// Set TEST_GIT_ROOT for !repo-root resolution
		t.Setenv("TEST_GIT_ROOT", absFixturesDir)
		// Set ATMOS_BASE_PATH with YAML function
		t.Setenv("ATMOS_BASE_PATH", "!repo-root")

		cmd := atmosRunner.Command("terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("ATMOS_BASE_PATH env var with literal path", func(t *testing.T) {
		// Test that literal paths still work in env var
		t.Setenv("ATMOS_BASE_PATH", absFixturesDir)

		cmd := atmosRunner.Command("terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("ATMOS_BASE_PATH env var with !env function", func(t *testing.T) {
		// Test environment variable indirection
		t.Setenv("MY_CUSTOM_INFRA_PATH", absFixturesDir)
		t.Setenv("ATMOS_BASE_PATH", "!env MY_CUSTOM_INFRA_PATH")

		cmd := atmosRunner.Command("terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("CLI flag overrides environment variable", func(t *testing.T) {
		// Both set, CLI flag should win
		t.Setenv("TEST_GIT_ROOT", absFixturesDir)
		t.Setenv("ATMOS_BASE_PATH", "!repo-root")

		// CLI flag with different path (literal) should override
		cmd := atmosRunner.Command("--base-path="+absFixturesDir, "terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("CLI flag with !repo-root overrides env var", func(t *testing.T) {
		t.Setenv("TEST_GIT_ROOT", absFixturesDir)
		t.Setenv("ATMOS_BASE_PATH", "/wrong/path")

		// CLI flag should override incorrect env var
		cmd := atmosRunner.Command("--base-path=!repo-root", "terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})
}

func TestDefaultBasePathRepoRoot(t *testing.T) {
	// Test that the embedded atmos.yaml has base_path: "!repo-root" as default
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	fixturesDir := filepath.Join("fixtures", "scenarios", "basic")
	absFixturesDir, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	t.Run("default behavior works from subdirectory", func(t *testing.T) {
		// Set TEST_GIT_ROOT to simulate git root
		t.Setenv("TEST_GIT_ROOT", absFixturesDir)

		// Change to component subdirectory
		componentDir := filepath.Join(absFixturesDir, "components", "terraform", "mycomponent")
		t.Chdir(componentDir)

		// Run command without any base_path overrides
		cmd := atmosRunner.Command("terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed because default base_path is !repo-root
		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found",
			"Default base_path should resolve to repo root")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("default behavior respects git root", func(t *testing.T) {
		t.Setenv("TEST_GIT_ROOT", absFixturesDir)

		// Run from fixtures directory
		t.Chdir(absFixturesDir)

		cmd := atmosRunner.Command("terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed - finds config at repo root
		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})
}
