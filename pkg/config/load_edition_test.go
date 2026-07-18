package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeEditionTestConfig writes an atmos.yaml into a temp dir, chdirs there, and
// neutralizes env vars that would otherwise shadow the defaults under test
// (PAGER is commonly set in developer shells and binds to settings.terminal.pager).
func writeEditionTestConfig(t *testing.T, yaml string) {
	t.Helper()
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, AtmosConfigFileName), []byte(yaml), 0o644))
	t.Chdir(tmpDir)
	for _, envVar := range []string{"PAGER", "ATMOS_PAGER", "ATMOS_EDITION", "ATMOS_LOGS_LEVEL", "ATMOS_LOGS_FILE"} {
		t.Setenv(envVar, "")
		require.NoError(t, os.Unsetenv(envVar))
	}
}

func TestLoadConfigEditionPin(t *testing.T) {
	// Compile-time sentinels: these tests reference specific schema fields.
	_ = schema.Helmfile{UseEKS: false}
	_ = schema.Terminal{Pager: ""}
	_ = schema.Logs{Level: "", File: ""}

	tests := []struct {
		name           string
		yaml           string
		wantUseEKS     bool
		wantPager      string
		wantLogsLevel  string
		wantLogsFile   string
		wantProvenance bool
	}{
		{
			name:           "no pin gets current defaults",
			yaml:           "base_path: ./\n",
			wantUseEKS:     false,
			wantPager:      "false",
			wantLogsLevel:  "Warning",
			wantLogsFile:   "/dev/stderr",
			wantProvenance: true,
		},
		{
			name:           "pin before the use_eks change restores the old default",
			yaml:           "base_path: ./\nedition: \"2026-01\"\n",
			wantUseEKS:     true,
			wantPager:      "false",
			wantLogsLevel:  "Warning",
			wantLogsFile:   "/dev/stderr",
			wantProvenance: false,
		},
		{
			name:           "earlier pin rolls back the pager and use_eks but keeps later-anchored logging",
			yaml:           "base_path: ./\nedition: \"2025-09\"\n",
			wantUseEKS:     true,
			wantPager:      "true",
			wantLogsLevel:  "Warning", // The logs.level change (2025-09-23) is inside the 2025-09 edition.
			wantLogsFile:   "/dev/stderr",
			wantProvenance: false,
		},
		{
			name:           "pin before the logs.level change restores Info",
			yaml:           "base_path: ./\nedition: \"2025-08\"\n",
			wantUseEKS:     true,
			wantPager:      "true",
			wantLogsLevel:  "Info",
			wantLogsFile:   "/dev/stderr",
			wantProvenance: false,
		},
		{
			name:           "pin before the logs.file change restores stdout logging",
			yaml:           "base_path: ./\nedition: \"2025-01\"\n",
			wantUseEKS:     true,
			wantPager:      "true",
			wantLogsLevel:  "Info",
			wantLogsFile:   "/dev/stdout",
			wantProvenance: false,
		},
		{
			name:           "year pin includes every change shipped that year",
			yaml:           "base_path: ./\nedition: \"2025\"\n",
			wantUseEKS:     true,
			wantPager:      "false",
			wantLogsLevel:  "Warning",
			wantLogsFile:   "/dev/stderr",
			wantProvenance: false,
		},
		{
			name:           "pin after all changes matches current defaults",
			yaml:           "base_path: ./\nedition: \"2026-07\"\n",
			wantUseEKS:     false,
			wantPager:      "false",
			wantLogsLevel:  "Warning",
			wantLogsFile:   "/dev/stderr",
			wantProvenance: true,
		},
		{
			name:           "explicit user value beats the pin",
			yaml:           "base_path: ./\nedition: \"2026-01\"\ncomponents:\n  helmfile:\n    use_eks: false\n",
			wantUseEKS:     false,
			wantPager:      "false",
			wantLogsLevel:  "Warning",
			wantLogsFile:   "/dev/stderr",
			wantProvenance: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeEditionTestConfig(t, tt.yaml)

			atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)

			assert.Equal(t, tt.wantUseEKS, atmosConfig.Components.Helmfile.UseEKS)
			assert.Equal(t, tt.wantPager, atmosConfig.Settings.Terminal.Pager)
			assert.Equal(t, tt.wantLogsLevel, atmosConfig.Logs.Level)
			assert.Equal(t, tt.wantLogsFile, atmosConfig.Logs.File)
			assert.Equal(t, tt.wantProvenance, atmosConfig.Describe.Provenance)
		})
	}
}

// TestLoadConfigEditionRollsBackJulyDefaults covers the July 2026 default flips
// (tree list formats, graceful error modes, help filter, provenance) and the
// December 2025 metadata-inheritance flip with one pre-July pin.
func TestLoadConfigEditionRollsBackJulyDefaults(t *testing.T) {
	_ = schema.StacksInherit{Metadata: nil}
	_ = schema.HelpSettings{Filter: false}

	t.Run("no pin gets the new defaults", func(t *testing.T) {
		writeEditionTestConfig(t, "base_path: ./\n")

		atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
		require.NoError(t, err)

		assert.Equal(t, "tree", atmosConfig.Stacks.List.Format)
		assert.Equal(t, "tree", atmosConfig.List.Instances.Format)
		assert.Equal(t, "warn", atmosConfig.List.ErrorMode)
		assert.Equal(t, "warn", atmosConfig.Describe.ErrorMode)
		assert.True(t, atmosConfig.Settings.Terminal.Help.Filter)
		assert.True(t, atmosConfig.Stacks.Inherit.IsMetadataInheritanceEnabled())
		assert.Equal(t, "schema", atmosConfig.Describe.Component.Filter)
	})

	t.Run("pin at 2026-07-01 rolls back everything after it", func(t *testing.T) {
		writeEditionTestConfig(t, "base_path: ./\nedition: \"2026-07-01\"\n")

		atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
		require.NoError(t, err)

		assert.Equal(t, "table", atmosConfig.Stacks.List.Format, "tree default (2026-07-16) must roll back to the explicit table format")
		assert.Equal(t, "table", atmosConfig.List.Instances.Format)
		assert.Equal(t, "strict", atmosConfig.List.ErrorMode, "graceful degradation (2026-07-13) must roll back to strict")
		assert.Equal(t, "strict", atmosConfig.Describe.ErrorMode)
		assert.False(t, atmosConfig.Settings.Terminal.Help.Filter, "help filter (2026-07-06) must roll back")
		assert.False(t, atmosConfig.Describe.Provenance, "provenance default (2026-07-16) must roll back")
		assert.Equal(t, "full", atmosConfig.Describe.Component.Filter, "output scope (2026-07-17) must roll back to full")
		assert.True(t, atmosConfig.Stacks.Inherit.IsMetadataInheritanceEnabled(),
			"metadata inheritance (2025-12-06) predates the pin and must stay on")
	})

	t.Run("pin before metadata inheritance restores per-component metadata", func(t *testing.T) {
		writeEditionTestConfig(t, "base_path: ./\nedition: \"2025-11\"\n")

		atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
		require.NoError(t, err)

		assert.False(t, atmosConfig.Stacks.Inherit.IsMetadataInheritanceEnabled())
	})
}

func TestLoadConfigEditionFromEnv(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\n")
	t.Setenv("ATMOS_EDITION", "2026-01")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.True(t, atmosConfig.Components.Helmfile.UseEKS, "ATMOS_EDITION must pin defaults")
	assert.Equal(t, "2026-01", atmosConfig.Edition)
}

func TestLoadConfigEditionEnvBeatsConfig(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\nedition: \"2026-07\"\n")
	t.Setenv("ATMOS_EDITION", "2026-01")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.True(t, atmosConfig.Components.Helmfile.UseEKS, "env pin must take precedence over the config pin")
	assert.Equal(t, "2026-01", atmosConfig.Edition)
}

func TestLoadConfigEditionInvalid(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\nedition: \"not-a-date\"\n")

	_, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.ErrorIs(t, err, edition.ErrInvalidEdition)
}

func TestLoadConfigEditionExposedOnConfig(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\nedition: \"2026\"\n")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.Equal(t, "2026", atmosConfig.Edition)
}

// TestLoadConfigEditionFromGlobalFlag covers resolveEditionPin's top-priority source:
// the --edition flag synced onto the global Viper singleton by syncGlobalFlagsToViper
// in cmd/root.go (simulated here directly, since that sync itself is a cmd-layer
// concern). It must win over both the config-file pin and (implicitly) ATMOS_EDITION.
func TestLoadConfigEditionFromGlobalFlag(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\nedition: \"2026-07\"\n")

	viper.GetViper().Set(editionKey, "2026-01")
	t.Cleanup(func() { viper.GetViper().Set(editionKey, "") })

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.Equal(t, "2026-01", atmosConfig.Edition, "the global --edition flag must win over the config-file pin")
	assert.True(t, atmosConfig.Components.Helmfile.UseEKS, "the flag-sourced pin must still apply its rollback overlay")
}

// TestLoadConfigEditionFromOsArgsFallback covers resolveEditionPin's os.Args fallback,
// used by commands that run with DisableFlagParsing=true (terraform, helmfile, packer,
// auth exec) where Cobra never populates the global Viper flag binding.
func TestLoadConfigEditionFromOsArgsFallback(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\n")

	origArgs := os.Args
	os.Args = []string{"atmos", "terraform", "plan", "--edition=2025-09"}
	t.Cleanup(func() { os.Args = origArgs })

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.Equal(t, "2025-09", atmosConfig.Edition, "os.Args fallback must resolve the pin when the flag isn't parsed by Cobra")
	assert.True(t, atmosConfig.Components.Helmfile.UseEKS)
}

func TestParseEditionFromOsArgs(t *testing.T) {
	assert.Equal(t, "2025-09", parseEditionFromOsArgs([]string{"terraform", "--edition=2025-09", "plan"}))
	assert.Equal(t, "2025-10", parseEditionFromOsArgs([]string{"helmfile", "--unknown", "x", "--edition", " 2025-10 "}))
	assert.Empty(t, parseEditionFromOsArgs([]string{"terraform", "plan", "--help"}))
}
