package tests

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestProfileListCommand verifies that profile list command works correctly.
func TestProfileListCommand(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for profile list test", "coverageEnabled", coverDir != "")
	}

	t.Run("profile list with profiles returns table output", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("profile", "list")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "profile list should succeed")

		output := stdout.String() + stderr.String()

		// Should show PROFILES header.
		assert.Contains(t, output, "PROFILES", "Should show PROFILES header")

		// Should show the profiles from test fixture.
		assert.Contains(t, output, "developer", "Should list developer profile")
		assert.Contains(t, output, "ci", "Should list ci profile")

		// Should show location type.
		assert.Contains(t, output, "project", "Should show project location type")

		// Should show table headers.
		assert.Contains(t, output, "NAME", "Should show NAME column")
		assert.Contains(t, output, "LOCATION", "Should show LOCATION column")
		assert.Contains(t, output, "PATH", "Should show PATH column")
		assert.Contains(t, output, "FILES", "Should show FILES column")
	})

	t.Run("profile list with no profiles shows notice", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("profile", "list")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "profile list should succeed even with no profiles")

		output := stdout.String() + stderr.String()

		// Should show empty state message.
		assert.Contains(t, output, "No profiles configured", "Should show empty state message")
	})

	t.Run("profile list json format", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("profile", "list", "--format", "json")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "profile list json should succeed")

		output := stdout.String()

		// Should be valid JSON.
		var profiles []map[string]interface{}
		err = json.Unmarshal([]byte(output), &profiles)
		require.NoError(t, err, "Output should be valid JSON")

		// Should have profiles.
		assert.NotEmpty(t, profiles, "Should have at least one profile")

		// Check profile structure.
		if len(profiles) > 0 {
			profile := profiles[0]
			assert.Contains(t, profile, "Name", "Profile should have Name field")
			assert.Contains(t, profile, "Path", "Profile should have Path field")
			assert.Contains(t, profile, "LocationType", "Profile should have LocationType field")
			assert.Contains(t, profile, "Files", "Profile should have Files field")
		}
	})

	t.Run("profile list yaml format", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("profile", "list", "--format", "yaml")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "profile list yaml should succeed")

		output := stdout.String()

		// Should contain YAML-style output.
		assert.Contains(t, output, "name:", "Should have YAML name field")
		assert.Contains(t, output, "path:", "Should have YAML path field")
		assert.Contains(t, output, "locationtype:", "Should have YAML locationtype field")
	})

	t.Run("atmos list profiles alias works", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("list", "profiles")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "list profiles should succeed")

		output := stdout.String() + stderr.String()

		// Should show PROFILES header.
		assert.Contains(t, output, "PROFILES", "Should show PROFILES header")

		// Should list profiles.
		assert.Contains(t, output, "developer", "Should list developer profile")
	})
}

// TestProfileShowCommand verifies that profile show command works correctly.
func TestProfileShowCommand(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for profile show test", "coverageEnabled", coverDir != "")
	}

	t.Run("profile show with existing profile", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("profile", "show", "developer")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "profile show should succeed")

		output := stdout.String() + stderr.String()

		// Should show profile header.
		assert.Contains(t, output, "PROFILE: developer", "Should show profile name header")

		// Should show location information.
		assert.Contains(t, output, "Location Type:", "Should show location type label")
		assert.Contains(t, output, "project", "Should show project location type")
		assert.Contains(t, output, "Path:", "Should show path label")

		// Should show FILES section.
		assert.Contains(t, output, "FILES", "Should show FILES header")

		// Should list configuration files.
		assert.Contains(t, output, "auth.yaml", "Should list auth.yaml file")
		assert.Contains(t, output, "settings.yaml", "Should list settings.yaml file")

		// Should show usage hint.
		assert.Contains(t, output, "Use with:", "Should show usage hint")
		assert.Contains(t, output, "atmos --profile developer", "Should show profile usage example")
	})

	t.Run("profile show with non-existent profile", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("profile", "show", "nonexistent")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.Error(t, err, "profile show should fail with non-existent profile")

		output := stdout.String() + stderr.String()

		// Should show error message.
		assert.Contains(t, output, "not found", "Should show not found error")
		assert.Contains(t, output, "nonexistent", "Should mention the profile name")

		// Should suggest using profile list.
		assert.Contains(t, output, "atmos profile list", "Should suggest profile list command")
	})

	t.Run("profile show json format", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("profile", "show", "developer", "--format", "json")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "profile show json should succeed")

		output := stdout.String()

		// Should be valid JSON.
		var profile map[string]interface{}
		err = json.Unmarshal([]byte(output), &profile)
		require.NoError(t, err, "Output should be valid JSON")

		// Check profile structure.
		assert.Equal(t, "developer", profile["Name"], "Profile name should be developer")
		assert.Contains(t, profile, "Path", "Profile should have Path field")
		assert.Contains(t, profile, "LocationType", "Profile should have LocationType field")
		assert.Contains(t, profile, "Files", "Profile should have Files field")

		// Check files array.
		files, ok := profile["Files"].([]interface{})
		require.True(t, ok, "Files should be an array")
		assert.NotEmpty(t, files, "Should have at least one file")
	})

	t.Run("profile show yaml format", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		cmd := atmosRunner.Command("profile", "show", "ci", "--format", "yaml")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		require.NoError(t, err, "profile show yaml should succeed")

		output := stdout.String()

		// Should contain YAML-style output.
		assert.Contains(t, output, "name: ci", "Should have YAML name field")
		assert.Contains(t, output, "path:", "Should have YAML path field")
		assert.Contains(t, output, "locationtype: project", "Should have YAML locationtype field")
		assert.Contains(t, output, "files:", "Should have YAML files field")
	})
}

// TestProfileFlagIntegration verifies that --profile flag is accepted by commands.
func TestProfileFlagIntegration(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for profile flag integration test", "coverageEnabled", coverDir != "")
	}

	t.Run("profile flag is accepted without error", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		// Profile flag should be accepted (profile loading is tested separately).
		cmd := atmosRunner.Command("profile", "list", "--profile", "developer")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed - profile flag is a global flag.
		require.NoError(t, err, "Commands with --profile flag should succeed")
	})

	t.Run("multiple profiles flag syntax works", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/config-profiles")

		// Multiple profiles should be accepted.
		cmd := atmosRunner.Command("profile", "list", "--profile", "ci", "--profile", "developer")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed - multiple profile flags are allowed.
		require.NoError(t, err, "Multiple --profile flags should be accepted")
	})
}
