package skill

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/config/homedir"
)

// captureStdout runs fn and returns everything it wrote to os.Stdout.
// A defer restores os.Stdout and closes the pipe so that an aborting fn()
// (e.g. require.*, panic, t.FailNow) cannot leave the process-global
// os.Stdout redirected or leak file descriptors.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = old }()

	// Drain the pipe concurrently so a large fn() output never blocks on a full
	// pipe buffer. Reading only after fn() returns deadlocks once the output
	// exceeds the OS pipe buffer (~64KB on Windows) — see the detailed skill
	// listing, which crosses that threshold with the bundled catalog.
	var buf bytes.Buffer
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(copied)
	}()

	fn()

	require.NoError(t, w.Close())
	<-copied
	_ = r.Close()
	return buf.String()
}

// withTempHome points HOME at a clean temp dir so the local skill registry
// starts empty, and resets the homedir cache for the duration of the test.
func withTempHome(t *testing.T) string {
	t.Helper()

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	// Windows resolves the home directory from USERPROFILE.
	t.Setenv("USERPROFILE", tempHome)

	homedir.Reset()
	homedir.DisableCache = true
	t.Cleanup(func() {
		homedir.Reset()
		homedir.DisableCache = false
	})

	return tempHome
}

// writeRegistry writes a registry.json under tempHome marking the given skills
// installed. Each entry is keyed by name with the provided source/version.
func writeRegistry(t *testing.T, tempHome string, skills map[string]map[string]interface{}) {
	t.Helper()

	skillsDir := filepath.Join(tempHome, ".atmos", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))

	registry := map[string]interface{}{
		"version": "1.0.0",
		"skills":  skills,
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "registry.json"), data, 0o600))
}

func installedEntry(name, source, version, path string) map[string]interface{} {
	now := time.Now().Format(time.RFC3339)
	return map[string]interface{}{
		"name":         name,
		"display_name": name,
		"source":       source,
		"version":      version,
		"installed_at": now,
		"updated_at":   now,
		"path":         path,
		"is_builtin":   false,
		"enabled":      true,
	}
}

// resetListFlags restores the list command's flags to defaults between subtests.
func resetListFlags(t *testing.T) {
	t.Helper()
	require.NoError(t, listCmd.Flags().Set("detailed", "false"))
	require.NoError(t, listCmd.Flags().Set("installed", "false"))
}

func TestListCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "list", listCmd.Use)
	assert.Equal(t, "List available and installed skills", listCmd.Short)
	assert.NotEmpty(t, listCmd.Long)
	assert.NotNil(t, listCmd.RunE)
	require.NotNil(t, listCmd.Args)
	assert.Error(t, listCmd.Args(listCmd, []string{"unexpected"}))
}

func TestListCmd_Flags(t *testing.T) {
	t.Run("detailed flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("detailed")
		require.NotNil(t, flag)
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
		assert.Equal(t, "d", flag.Shorthand)
	})

	t.Run("installed flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("installed")
		require.NotNil(t, flag)
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestListCmd_EnvVarBinding(t *testing.T) {
	t.Run("detailed env var", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_DETAILED", "true")
		v := viper.New()
		require.NoError(t, listParser.BindToViper(v))
		assert.True(t, v.GetBool("detailed"))
	})

	t.Run("installed env var", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_INSTALLED", "true")
		v := viper.New()
		require.NoError(t, listParser.BindToViper(v))
		assert.True(t, v.GetBool("installed"))
	})
}

func TestBuildListEntries(t *testing.T) {
	catalog, err := marketplace.Catalog()
	require.NoError(t, err)
	require.Positive(t, len(catalog), "bundled catalog must not be empty")

	t.Run("nothing installed: all catalog entries available, none installed", func(t *testing.T) {
		withTempHome(t)

		installer, err := marketplace.NewInstaller("test")
		require.NoError(t, err)

		entries, err := buildListEntries(installer)
		require.NoError(t, err)
		require.Len(t, entries, len(catalog))

		// Assert first and last by value, not just length.
		assert.Equal(t, catalog[0].Name, entries[0].name)
		assert.True(t, entries[0].available)
		assert.False(t, entries[0].installed)
		assert.Equal(t, catalog[len(catalog)-1].Name, entries[len(entries)-1].name)
		assert.True(t, entries[len(entries)-1].available)
		assert.False(t, entries[len(entries)-1].installed)
	})

	t.Run("installed catalog skill is marked installed with its version", func(t *testing.T) {
		tempHome := withTempHome(t)
		skillPath := filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform")
		writeRegistry(t, tempHome, map[string]map[string]interface{}{
			"atmos-terraform": installedEntry(
				"atmos-terraform",
				"github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform",
				"1.0.0",
				skillPath,
			),
		})

		installer, err := marketplace.NewInstaller("test")
		require.NoError(t, err)

		entries, err := buildListEntries(installer)
		require.NoError(t, err)
		require.Len(t, entries, len(catalog), "installed catalog skill should not add a duplicate row")

		var found bool
		for _, e := range entries {
			if e.name != "atmos-terraform" {
				continue
			}
			found = true
			assert.True(t, e.installed)
			assert.True(t, e.available)
			assert.Equal(t, "1.0.0", e.version)
			require.NotNil(t, e.skill)
		}
		assert.True(t, found, "atmos-terraform must be present")
	})

	t.Run("community skill not in catalog is appended as installed-only", func(t *testing.T) {
		tempHome := withTempHome(t)
		skillPath := filepath.Join(tempHome, ".atmos", "skills", "my-skill")
		writeRegistry(t, tempHome, map[string]map[string]interface{}{
			"my-skill": installedEntry("my-skill", "github.com/example/my-skill", "v2.0.0", skillPath),
		})

		installer, err := marketplace.NewInstaller("test")
		require.NoError(t, err)

		entries, err := buildListEntries(installer)
		require.NoError(t, err)
		require.Len(t, entries, len(catalog)+1)

		var found bool
		for _, e := range entries {
			if e.name != "my-skill" {
				continue
			}
			found = true
			assert.True(t, e.installed)
			assert.False(t, e.available)
			assert.Equal(t, "v2.0.0", e.version)
		}
		assert.True(t, found, "community skill must be appended")
	})
}

func TestListCmd_DefaultOutput(t *testing.T) {
	catalog, err := marketplace.Catalog()
	require.NoError(t, err)

	tempHome := withTempHome(t)
	skillPath := filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform")
	writeRegistry(t, tempHome, map[string]map[string]interface{}{
		"atmos-terraform": installedEntry(
			"atmos-terraform",
			"github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform",
			"1.0.0",
			skillPath,
		),
	})
	resetListFlags(t)

	output := captureStdout(t, func() {
		require.NoError(t, listCmd.RunE(listCmd, []string{}))
	})

	// Header reflects available + installed counts.
	assert.Contains(t, output, "Atmos skills (")
	assert.Contains(t, output, "available")
	assert.Contains(t, output, "installed")
	// Installed skill gets the filled marker; an available one gets the hollow marker.
	assert.Contains(t, output, markerInstalled+" atmos-terraform")
	assert.Contains(t, output, markerAvailable)
	// Legend + install hint.
	assert.Contains(t, output, "atmos ai skill install <name>")
	// Every catalog skill is listed.
	for _, c := range catalog {
		assert.Contains(t, output, c.Name)
	}
}

func TestListCmd_InstalledOnly(t *testing.T) {
	t.Run("with an installed skill shows only it", func(t *testing.T) {
		tempHome := withTempHome(t)
		skillPath := filepath.Join(tempHome, ".atmos", "skills", "atmos-stacks")
		writeRegistry(t, tempHome, map[string]map[string]interface{}{
			"atmos-stacks": installedEntry(
				"atmos-stacks",
				"github.com/cloudposse/atmos//agent-skills/skills/atmos-stacks",
				"1.0.0",
				skillPath,
			),
		})
		resetListFlags(t)
		require.NoError(t, listCmd.Flags().Set("installed", "true"))

		output := captureStdout(t, func() {
			require.NoError(t, listCmd.RunE(listCmd, []string{}))
		})

		assert.Contains(t, output, "atmos-stacks")
		// A skill that is available-but-not-installed must be absent.
		assert.NotContains(t, output, "atmos-vendoring")
	})

	t.Run("with nothing installed shows empty message", func(t *testing.T) {
		withTempHome(t)
		resetListFlags(t)
		require.NoError(t, listCmd.Flags().Set("installed", "true"))

		output := captureStdout(t, func() {
			require.NoError(t, listCmd.RunE(listCmd, []string{}))
		})

		assert.Contains(t, output, "No skills installed")
		assert.Contains(t, output, "atmos ai skill list")
	})
}

func TestListCmd_DetailedOutput(t *testing.T) {
	tempHome := withTempHome(t)
	skillPath := filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform")
	writeRegistry(t, tempHome, map[string]map[string]interface{}{
		"atmos-terraform": installedEntry(
			"atmos-terraform",
			"github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform",
			"1.0.0",
			skillPath,
		),
	})
	resetListFlags(t)
	require.NoError(t, listCmd.Flags().Set("detailed", "true"))

	output := captureStdout(t, func() {
		require.NoError(t, listCmd.RunE(listCmd, []string{}))
	})

	assert.Contains(t, output, "━━━")
	assert.Contains(t, output, "Name:")
	assert.Contains(t, output, "Version:")
	assert.Contains(t, output, "Source:")
	// Both statuses appear: the installed skill and the available ones.
	assert.Contains(t, output, "(Installed)")
	assert.Contains(t, output, "(Available)")
	// Install details only render for installed skills.
	assert.Contains(t, output, "Location:")
}

func TestListCmd_CorruptedRegistry(t *testing.T) {
	tempHome := withTempHome(t)
	skillsDir := filepath.Join(tempHome, ".atmos", "skills")
	require.NoError(t, os.MkdirAll(skillsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillsDir, "registry.json"), []byte("not json"), 0o600))
	resetListFlags(t)

	err := listCmd.RunE(listCmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize installer")
}

func TestCountEntries(t *testing.T) {
	entries := []listEntry{
		{name: "a", available: true, installed: true},  // Catalog skill that is already installed.
		{name: "b", available: true, installed: false}, // Catalog skill not yet installed.
		{name: "c", available: false, installed: true}, // Community install (not in catalog).
	}
	available, installed := countEntries(entries)
	// "available" counts only uninstalled catalog rows (hollow-dot entries).
	// Entry a is already installed so it does NOT count as available.
	assert.Equal(t, 1, available)
	assert.Equal(t, 2, installed)
}

func TestFormatTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"1 minute ago", now.Add(-1 * time.Minute), "1 minute ago"},
		{"5 minutes ago", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1 hour ago"},
		{"3 hours ago", now.Add(-3 * time.Hour), "3 hours ago"},
		{"yesterday", now.Add(-25 * time.Hour), "yesterday"},
		{"3 days ago", now.Add(-3 * 24 * time.Hour), "3 days ago"},
		{"more than a week ago", now.Add(-10 * 24 * time.Hour), now.Add(-10 * 24 * time.Hour).Format("Jan 2, 2006")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatTime(tt.time))
		})
	}
}

func TestFormatTime_SpecificDate(t *testing.T) {
	oldDate := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	assert.Contains(t, formatTime(oldDate), "Jun 15, 2024")
}

func TestListCmd_Examples(t *testing.T) {
	assert.Contains(t, listCmd.Example, "atmos ai skill list")
}
