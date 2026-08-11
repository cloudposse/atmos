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

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

type testStreams struct {
	input  io.Reader
	output io.Writer
	error  io.Writer
}

func (s testStreams) Input() io.Reader {
	return s.input
}

func (s testStreams) Output() io.Writer {
	return s.output
}

func (s testStreams) Error() io.Writer {
	return s.error
}

func (s testStreams) RawOutput() io.Writer {
	return s.output
}

func (s testStreams) RawError() io.Writer {
	return s.error
}

func setupSkillListOutput(t *testing.T) *bytes.Buffer {
	t.Helper()

	iolib.Reset()
	data.Reset()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	ioCtx, err := iolib.NewContext(iolib.WithStreams(testStreams{
		input:  bytes.NewBuffer(nil),
		output: &stdout,
		error:  &stderr,
	}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	t.Cleanup(func() {
		data.Reset()
		iolib.Reset()
	})

	return &stdout
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

func installedEntryWithMetadata(name, source, version, path string, enabled, isBuiltIn bool) map[string]interface{} {
	entry := installedEntry(name, source, version, path)
	entry["enabled"] = enabled
	entry["is_builtin"] = isBuiltIn
	return entry
}

// installedEntryWithMinAtmos is installedEntry plus a compatibility.atmos
// constraint recorded at install time, for exercising `skill list --detailed`'s
// "Min Atmos" line.
func installedEntryWithMinAtmos(name, source, version, path, minAtmosVersion string) map[string]interface{} {
	entry := installedEntry(name, source, version, path)
	entry["min_atmos_version"] = minAtmosVersion
	return entry
}

// resetListFlags restores the list command's flags to defaults between subtests,
// clearing pflag's "Changed" state too. Without clearing Changed, a flag stays
// "explicitly changed" forever once any earlier subtest calls Set() on it (pflag
// has no unset), which would make Viper always prefer the stale CLI value over
// an env var in later subtests that test env-var precedence.
func resetListFlags(t *testing.T) {
	t.Helper()
	flags.ResetCommandFlags(listCmd)
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

	t.Run("format flag", func(t *testing.T) {
		flag := listCmd.Flags().Lookup(flagFormat)
		require.NotNil(t, flag)
		assert.Equal(t, "string", flag.Value.Type())
		assert.Equal(t, "", flag.DefValue)
		assert.Equal(t, "f", flag.Shorthand)
	})
}

func TestSkillListColumns(t *testing.T) {
	columns := skillListColumns()
	require.Len(t, columns, 5)
	assert.Equal(t, " ", columns[0].Name)
	assert.Equal(t, 1, columns[0].Width)
	assert.Equal(t, "Name", columns[1].Name)
	assert.Equal(t, "Source", columns[2].Name)
	assert.Equal(t, "State", columns[3].Name)
	assert.Equal(t, "Category", columns[4].Name)
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

	t.Run("format env var", func(t *testing.T) {
		t.Setenv("ATMOS_AI_SKILL_FORMAT", "json")
		v := viper.New()
		require.NoError(t, listParser.BindToViper(v))
		assert.Equal(t, "json", v.GetString(flagFormat))
	})
}

// TestListCmd_FormatEnvVarInvalid verifies that a value sourced from
// ATMOS_AI_SKILL_FORMAT (rather than the --format flag itself) is still
// validated: the flag parser only validates values that came directly from
// an explicitly-changed CLI flag, so RunE must check the Viper-resolved
// value before it reaches the renderer.
func TestListCmd_FormatEnvVarInvalid(t *testing.T) {
	withTempHome(t)
	resetListFlags(t)
	t.Setenv("ATMOS_AI_SKILL_FORMAT", "invalid")

	// Set up output plumbing in case validation regresses and the command falls
	// through to rendering; without this a regression would panic instead of
	// failing the assertions below with a clear message.
	setupSkillListOutput(t)

	err := listCmd.RunE(listCmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Contains(t, err.Error(), "invalid")
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
		assert.Equal(t, sourceBuiltIn, entries[0].displaySource)
		assert.Equal(t, catalog[0].Source, entries[0].source)
		assert.Equal(t, catalog[0].Category, entries[0].category)
		assert.Equal(t, catalog[len(catalog)-1].Name, entries[len(entries)-1].name)
		assert.True(t, entries[len(entries)-1].available)
		assert.False(t, entries[len(entries)-1].installed)
		assert.Equal(t, sourceBuiltIn, entries[len(entries)-1].displaySource)
		assert.Equal(t, catalog[len(catalog)-1].Source, entries[len(entries)-1].source)
		assert.Equal(t, catalog[len(catalog)-1].Category, entries[len(entries)-1].category)
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
			assert.Equal(t, sourceBuiltIn, e.displaySource)
			assert.Equal(t, "github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform", e.source)
			require.NotNil(t, e.skill)
			assert.True(t, e.skill.Enabled)
			assert.False(t, e.skill.IsBuiltIn)
			assert.False(t, e.updateAvailable, "installed version matches the catalog's current version")
		}
		assert.True(t, found, "atmos-terraform must be present")
	})

	t.Run("installed catalog skill with an older version is marked updateAvailable", func(t *testing.T) {
		tempHome := withTempHome(t)
		skillPath := filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform")
		writeRegistry(t, tempHome, map[string]map[string]interface{}{
			"atmos-terraform": installedEntry(
				"atmos-terraform",
				"github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform",
				"0.9.0", // Older than the real catalog's "1.0.0".
				skillPath,
			),
		})

		installer, err := marketplace.NewInstaller("test")
		require.NoError(t, err)

		entries, err := buildListEntries(installer)
		require.NoError(t, err)

		var found bool
		for _, e := range entries {
			if e.name != "atmos-terraform" {
				continue
			}
			found = true
			assert.True(t, e.updateAvailable)
			assert.Equal(t, "0.9.0", e.version)
			assert.Equal(t, "1.0.0", e.catalogVersion)
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
			assert.Equal(t, "github.com/example/my-skill", e.displaySource)
			assert.Equal(t, "github.com/example/my-skill", e.source)
		}
		assert.True(t, found, "community skill must be appended")
	})

	t.Run("installed metadata is preserved for disabled built-in skills", func(t *testing.T) {
		tempHome := withTempHome(t)
		skillPath := filepath.Join(tempHome, ".atmos", "skills", "custom-builtin")
		writeRegistry(t, tempHome, map[string]map[string]interface{}{
			"custom-builtin": installedEntryWithMetadata(
				"custom-builtin",
				sourceBuiltIn,
				"v3.0.0",
				skillPath,
				false,
				true,
			),
		})

		installer, err := marketplace.NewInstaller("test")
		require.NoError(t, err)

		entries, err := buildListEntries(installer)
		require.NoError(t, err)

		var found bool
		for _, e := range entries {
			if e.name != "custom-builtin" {
				continue
			}
			found = true
			require.NotNil(t, e.skill)
			assert.False(t, e.skill.Enabled)
			assert.True(t, e.skill.IsBuiltIn)
		}
		assert.True(t, found, "custom-builtin must be present")
	})
}

func TestSkillListRows_InstalledState(t *testing.T) {
	rows := skillListRows([]listEntry{
		{name: "available", displaySource: sourceBuiltIn},
		{
			name:          "enabled-skill",
			version:       "1.0.0",
			displaySource: "github.com/example/enabled",
			installed:     true,
			skill:         &marketplace.InstalledSkill{Enabled: true},
		},
		{
			name:          "disabled-skill",
			version:       "v2.0.0",
			displaySource: "github.com/example/disabled",
			installed:     true,
			skill:         &marketplace.InstalledSkill{Enabled: false},
		},
		{
			name:            "outdated-skill",
			version:         "0.9.0",
			catalogVersion:  "1.0.0",
			displaySource:   "github.com/example/outdated",
			installed:       true,
			updateAvailable: true,
			skill:           &marketplace.InstalledSkill{Enabled: true},
		},
	})

	require.Len(t, rows, 4)
	assert.Equal(t, "available", rows[0]["state"])
	assert.Equal(t, "installed, enabled (v1.0.0)", rows[1]["state"])
	assert.Equal(t, "installed, disabled (v2.0.0)", rows[2]["state"])
	assert.Equal(t, "installed, enabled (v0.9.0) (update available)", rows[3]["state"])
}

func TestRenderEntryDetails_Metadata(t *testing.T) {
	now := time.Now()
	output := renderEntryDetails([]listEntry{
		{
			name:        "atmos-terraform",
			displayName: "Atmos Terraform",
			version:     "1.0.0",
			source:      "github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform",
			available:   true,
			installed:   true,
			skill: &marketplace.InstalledSkill{
				Enabled:     true,
				IsBuiltIn:   false,
				InstalledAt: now,
				UpdatedAt:   now,
				Path:        "/tmp/atmos-terraform",
			},
		},
		{
			name:        "community-skill",
			displayName: "Community Skill",
			version:     "v2.0.0",
			source:      "github.com/example/community-skill",
			installed:   true,
			skill: &marketplace.InstalledSkill{
				Enabled:     false,
				IsBuiltIn:   false,
				InstalledAt: now,
				UpdatedAt:   now,
				Path:        "/tmp/community-skill",
			},
		},
		{
			name:        "available-skill",
			displayName: "Available Skill",
			version:     "1.0.0",
			source:      "github.com/cloudposse/atmos//agent-skills/skills/available-skill",
			available:   true,
		},
		{
			name:            "outdated-skill",
			displayName:     "Outdated Skill",
			version:         "0.9.0",
			catalogVersion:  "1.0.0",
			source:          "github.com/cloudposse/atmos//agent-skills/skills/outdated-skill",
			available:       true,
			installed:       true,
			updateAvailable: true,
			skill: &marketplace.InstalledSkill{
				Enabled:         true,
				InstalledAt:     now,
				UpdatedAt:       now,
				Path:            "/tmp/outdated-skill",
				MinAtmosVersion: "1.2.0",
			},
		},
	})

	assert.Contains(t, output, "Atmos Terraform (Installed)")
	assert.Contains(t, output, "Status:       Enabled")
	assert.Contains(t, output, "Community Skill (Installed)")
	assert.Contains(t, output, "Status:       Disabled")
	assert.Contains(t, output, "Type:         Built-in")
	assert.Contains(t, output, "Type:         Community")
	assert.Contains(t, output, "Available Skill (Available)")

	// Compatibility constraint recorded at install time.
	assert.Contains(t, output, "Min Atmos:    1.2.0")

	// Update-available note on the Version line, pointing at the catalog's
	// current version.
	assert.Contains(t, output, "Version:      0.9.0 (update available: 1.0.0)")
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

	stdout := setupSkillListOutput(t)
	require.NoError(t, listCmd.RunE(listCmd, []string{}))
	output := stdout.String()

	// Header reflects available + installed counts.
	assert.Contains(t, output, "Atmos skills (")
	assert.Contains(t, output, "available")
	assert.Contains(t, output, "installed")
	// Installed skill gets the filled marker; an available one gets the hollow marker.
	assert.Contains(t, output, markerInstalled+"\tatmos-terraform\t"+sourceBuiltIn)
	assert.Contains(t, output, markerAvailable)
	// Legend + install hint.
	assert.Contains(t, output, "Install a built-in skill by name:")
	assert.Contains(t, output, "atmos ai skill install <name>")
	assert.Contains(t, output, "Install from a repository source:")
	assert.Contains(t, output, "atmos ai skill install <source>")
	// Every catalog skill is listed.
	for _, c := range catalog {
		assert.Contains(t, output, c.Name)
	}
}

func TestListCmd_FormatJSON(t *testing.T) {
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
	require.NoError(t, listCmd.Flags().Set(flagFormat, "json"))

	stdout := setupSkillListOutput(t)
	require.NoError(t, listCmd.RunE(listCmd, []string{}))
	output := stdout.String()

	// The human-oriented header, legend, and install hint must not appear.
	assert.NotContains(t, output, "Atmos skills (")
	assert.NotContains(t, output, "Install a built-in skill by name:")

	var rows []map[string]string
	require.NoError(t, json.Unmarshal([]byte(output), &rows), "output must be valid JSON")
	require.Len(t, rows, len(catalog))

	var found bool
	for _, row := range rows {
		assert.Contains(t, row, "Name")
		assert.Contains(t, row, "Source")
		assert.Contains(t, row, "State")
		assert.Contains(t, row, "Category")
		if row["Name"] == "atmos-terraform" {
			found = true
			assert.NotEmpty(t, row["Category"])
		}
	}
	assert.True(t, found, "atmos-terraform must be present in JSON output")
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

		stdout := setupSkillListOutput(t)
		require.NoError(t, listCmd.RunE(listCmd, []string{}))
		output := stdout.String()

		assert.Contains(t, output, "atmos-stacks")
		// A skill that is available-but-not-installed must be absent.
		assert.NotContains(t, output, "atmos-vendoring")
	})

	t.Run("with nothing installed shows empty message", func(t *testing.T) {
		withTempHome(t)
		resetListFlags(t)
		require.NoError(t, listCmd.Flags().Set("installed", "true"))

		stdout := setupSkillListOutput(t)
		require.NoError(t, listCmd.RunE(listCmd, []string{}))
		output := stdout.String()

		assert.Contains(t, output, "No skills installed")
		assert.Contains(t, output, "atmos ai skill list")
	})

	t.Run("with nothing installed and format=json returns an empty array, not prose", func(t *testing.T) {
		withTempHome(t)
		resetListFlags(t)
		require.NoError(t, listCmd.Flags().Set("installed", "true"))
		require.NoError(t, listCmd.Flags().Set(flagFormat, "json"))

		stdout := setupSkillListOutput(t)
		require.NoError(t, listCmd.RunE(listCmd, []string{}))
		output := stdout.String()

		assert.NotContains(t, output, "No skills installed")

		var rows []map[string]string
		require.NoError(t, json.Unmarshal([]byte(output), &rows), "output must be valid JSON")
		require.NotNil(t, rows, "output must be a JSON array, not null")
		assert.Empty(t, rows)
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

	stdout := setupSkillListOutput(t)
	require.NoError(t, listCmd.RunE(listCmd, []string{}))
	output := stdout.String()

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

// TestListCmd_DetailedOutput_ShowsMinAtmosVersion covers rendering
// compatibility.atmos, recorded at install time as InstalledSkill.MinAtmosVersion,
// in `skill list --detailed`.
func TestListCmd_DetailedOutput_ShowsMinAtmosVersion(t *testing.T) {
	tempHome := withTempHome(t)
	skillPath := filepath.Join(tempHome, ".atmos", "skills", "atmos-terraform")
	writeRegistry(t, tempHome, map[string]map[string]interface{}{
		"atmos-terraform": installedEntryWithMinAtmos(
			"atmos-terraform",
			"github.com/cloudposse/atmos//agent-skills/skills/atmos-terraform",
			"1.0.0",
			skillPath,
			"1.5.0",
		),
	})
	resetListFlags(t)
	require.NoError(t, listCmd.Flags().Set("detailed", "true"))

	stdout := setupSkillListOutput(t)
	require.NoError(t, listCmd.RunE(listCmd, []string{}))
	output := stdout.String()

	assert.Contains(t, output, "Min Atmos:")
	assert.Contains(t, output, "1.5.0")
}

// TestListCmd_DetailedOutput_OmitsMinAtmosVersionWhenUnset covers the common case:
// most installed skills declare no compatibility.atmos constraint, so the "Min
// Atmos" line must not appear at all (not even blank) for them.
func TestListCmd_DetailedOutput_OmitsMinAtmosVersionWhenUnset(t *testing.T) {
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

	stdout := setupSkillListOutput(t)
	require.NoError(t, listCmd.RunE(listCmd, []string{}))
	output := stdout.String()

	assert.NotContains(t, output, "Min Atmos:")
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

	// The underlying registry-corruption error must still carry its actionable
	// hints after being wrapped by NewInstaller/RunE, not just the bare message.
	registryPath := filepath.Join(skillsDir, "registry.json")
	assert.True(t, errUtils.HasHint(err, "rm "+registryPath),
		"hint should tell the user how to delete/repair the corrupted registry file")
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
