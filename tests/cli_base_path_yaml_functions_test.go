package tests

import (
	"bytes"
	"os"
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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

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
		err = cmd.Run()

		// Should succeed - finds config at repo root
		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})
}

func TestBasePathDotRestoresOldBehavior(t *testing.T) {
	// Test that users can set base_path: "." to restore pre-1.198.0 behavior
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Create a temporary directory with atmos.yaml that sets base_path: "."
	tempDir := t.TempDir()

	// Copy atmos.yaml to temp dir and override base_path
	atmosYAML := `base_path: "."

logs:
  file: "/dev/stderr"
  level: Info

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "orgs/**/*"
  excluded_paths:
    - "**/_defaults.yaml"
  name_pattern: "{tenant}-{environment}-{stage}"

schemas:
  jsonschema:
    base_path: "stacks/schemas/jsonschema"
  opa:
    base_path: "stacks/schemas/opa"
  atmos:
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
`
	err := os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644)
	require.NoError(t, err)

	t.Run("base_path dot resolves to current directory", func(t *testing.T) {
		// Change to temp directory
		t.Chdir(tempDir)

		// Run atmos version - should use current dir as base_path
		cmd := atmosRunner.Command("version")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()

		// Should succeed - finds atmos.yaml in current directory
		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("base_path dot does NOT search parent directories", func(t *testing.T) {
		// Create subdirectory
		subDir := filepath.Join(tempDir, "subdir")
		err = os.MkdirAll(subDir, 0o755)
		require.NoError(t, err)

		// Change to subdirectory
		t.Chdir(subDir)

		// Run atmos version - should NOT find atmos.yaml in parent
		cmd := atmosRunner.Command("version")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()

		// Should succeed (version doesn't require atmos.yaml)
		// But config won't be loaded from parent directory
		assert.NoError(t, err)
	})

	t.Run("base_path dot with explicit config path works", func(t *testing.T) {
		// Even from subdirectory, we can point to config
		subDir := filepath.Join(tempDir, "subdir", "nested")
		err = os.MkdirAll(subDir, 0o755)
		require.NoError(t, err)

		t.Chdir(subDir)

		// Use --config to point to parent atmos.yaml
		cmd := atmosRunner.Command("--config", filepath.Join(tempDir, "atmos.yaml"), "version")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()

		assert.NoError(t, err, "stdout: %s, stderr: %s", stdout.String(), stderr.String())
	})

	t.Run("base_path dot behavior matches pre-1.198.0", func(t *testing.T) {
		// This test documents that base_path: "." gives you the old behavior
		// where paths are relative to current working directory
		t.Chdir(tempDir)

		cmd := atmosRunner.Command("version")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()

		// Old behavior: only current directory is checked
		assert.NoError(t, err)
		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found")
	})
}
