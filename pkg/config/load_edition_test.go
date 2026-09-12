package config

import (
	"fmt"
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
// (graceful error modes, help filter, provenance, component filter) and the
// December 2025 metadata-inheritance flip with one pre-July pin.
func TestLoadConfigEditionRollsBackJulyDefaults(t *testing.T) {
	_ = schema.StacksInherit{Metadata: nil}
	_ = schema.HelpSettings{Filter: false}

	t.Run("no pin gets the new defaults", func(t *testing.T) {
		writeEditionTestConfig(t, "base_path: ./\n")

		atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
		require.NoError(t, err)

		// Unset ("" — renderer.formatTable treats empty as table); these two keys have
		// no Viper default, matching pre-edition behavior. Not journaled: nothing to roll back.
		assert.Empty(t, atmosConfig.Stacks.List.Format)
		assert.Empty(t, atmosConfig.List.Instances.Format)
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

// TestLoadConfigEditionInitModeAndUpgrade covers components.terraform.init.mode and
// init.upgrade's journal entries (both dated 2026-09-12): brand-new keys that still needed a
// journal entry, because their defaults govern behavior (running init at all; -upgrade
// automation) Atmos always had a fixed, unconfigurable answer for before these settings existed
// -- see pkg/edition/journal.go's comments on these entries and
// schema.Terraform.EffectiveInitMode/EffectiveInitUpgrade's doc comments. Table-driven across
// both keys since the pin-resolution behavior under test is identical; only the field being
// read and its pre-edition value ("always" vs "never") differ.
func TestLoadConfigEditionInitModeAndUpgrade(t *testing.T) {
	_ = schema.TerraformInit{Mode: "", Upgrade: ""}

	type field struct {
		key        string // atmos.yaml key under components.terraform.init
		preEdition string // value a pre-2026-09-12 pin restores
		get        func(schema.AtmosConfiguration) string
	}
	fields := []field{
		{
			key:        "mode",
			preEdition: "always",
			get:        func(c schema.AtmosConfiguration) string { return string(c.Components.Terraform.Init.Mode) },
		},
		{
			key:        "upgrade",
			preEdition: "never",
			get:        func(c schema.AtmosConfiguration) string { return string(c.Components.Terraform.Init.Upgrade) },
		},
	}

	tests := []struct {
		name string
		yaml func(f field) string
		want func(f field) string
	}{
		{
			name: "no pin gets the new auto default",
			yaml: func(field) string { return "base_path: ./\n" },
			want: func(field) string { return "auto" },
		},
		{
			name: "pin on the release date gets auto",
			yaml: func(field) string { return "base_path: ./\nedition: \"2026-09-12\"\n" },
			want: func(field) string { return "auto" },
		},
		{
			name: "pin one day before the release date restores the pre-edition value",
			yaml: func(field) string { return "base_path: ./\nedition: \"2026-09-11\"\n" },
			want: func(f field) string { return f.preEdition },
		},
		{
			name: "an old pin also restores the pre-edition value",
			yaml: func(field) string { return "base_path: ./\nedition: \"2026-01\"\n" },
			want: func(f field) string { return f.preEdition },
		},
		{
			name: "explicit user value beats the pin",
			yaml: func(f field) string {
				return fmt.Sprintf("base_path: ./\nedition: \"2026-01\"\ncomponents:\n  terraform:\n    init:\n      %s: always\n", f.key)
			},
			want: func(field) string { return "always" },
		},
	}

	for _, f := range fields {
		t.Run(f.key, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					writeEditionTestConfig(t, tt.yaml(f))

					atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
					require.NoError(t, err)

					assert.Equal(t, tt.want(f), f.get(atmosConfig))
				})
			}
		})
	}
}

// TestLoadConfigEditionInitReconfigureIsNotPinProtected documents, with a real end-to-end
// assertion rather than just a code comment, that init.reconfigure deliberately does NOT get
// the same edition-pin protection as init.mode/init.upgrade: an old pin does not restore
// "always" here, because init.reconfigure has no Viper default (see the comment at its
// SetDefault call site in pkg/config/load.go) -- its legacy init_run_reconfigure fallback
// requires t.Init.Reconfigure to stay genuinely unset when the user hasn't set it explicitly.
// If this test ever starts failing because init.reconfigure now resolves to "always" under an
// old pin, that means someone added a SetDefault for it -- which silently breaks
// init_run_reconfigure: false for any project relying on that legacy mapping; see
// EffectiveInitReconfigure's doc comment before doing that.
func TestLoadConfigEditionInitReconfigureIsNotPinProtected(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\nedition: \"2026-01\"\n")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.Equal(t, "auto", string(atmosConfig.Components.Terraform.EffectiveInitReconfigure()),
		"init.reconfigure is not edition-pin-protected; an old pin still resolves to auto via the legacy init_run_reconfigure fallback")
}

// TestLoadConfigNoAtmosYamlDefaults exercises the fallback path taken when no
// atmos.yaml is discoverable at all (mergeDefaultConfig / defaultCliConfig),
// as opposed to writeEditionTestConfig's tests, which always provide one and
// so never touch this path. This is the exact path that let --help regress to
// showing everything by default: defaultCliConfig didn't state Help.Filter, so
// its zero value (false) silently overrode the true SetDefault ships.
func TestLoadConfigNoAtmosYamlDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	for _, envVar := range []string{"PAGER", "ATMOS_PAGER", "ATMOS_EDITION", "ATMOS_LOGS_LEVEL", "ATMOS_LOGS_FILE", "ATMOS_HELP_FILTER"} {
		t.Setenv(envVar, "")
		require.NoError(t, os.Unsetenv(envVar))
	}

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.True(t, atmosConfig.Settings.Terminal.Help.Filter,
		"bare --help must default to the focused view even with no atmos.yaml discoverable")
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

	origEdition := viper.GetViper().Get(editionKey)
	viper.GetViper().Set(editionKey, "2026-01")
	t.Cleanup(func() { viper.GetViper().Set(editionKey, origEdition) })

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

// TestEditionPinSource covers EditionPinSource's precedence tiers directly —
// it shares resolveEditionPin's own flag-detection (editionPinFromFlag), so
// `atmos describe edition` (cmd/describe_edition.go) never needs its own copy
// of this precedence. EditionPinSource takes the already-resolved pin value
// (as cmd/describe_edition.go has it via atmosConfig.Edition) rather than a
// Viper instance — the caller's local Viper instance (the one LoadConfig used
// to merge the config file) isn't available in cmd/, only the global one.
func TestEditionPinSource(t *testing.T) {
	t.Run("no pin has no source", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "")
		require.NoError(t, os.Unsetenv("ATMOS_EDITION"))
		assert.Empty(t, EditionPinSource(""))
	})

	t.Run("global flag wins over everything", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "2026")
		viper.GetViper().Set(editionKey, "2026-07")
		t.Cleanup(func() { viper.GetViper().Set(editionKey, "") })

		assert.Equal(t, "flag", EditionPinSource("2026-07"))
	})

	t.Run("os.Args fallback counts as flag", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "2026")
		origArgs := os.Args
		os.Args = []string{"atmos", "terraform", "plan", "--edition=2025-09"}
		t.Cleanup(func() { os.Args = origArgs })

		assert.Equal(t, "flag", EditionPinSource("2025-09"))
	})

	t.Run("env when neither flag nor os.Args fallback is set", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "2026")
		assert.Equal(t, "env", EditionPinSource("2026"))
	})

	t.Run("config when neither flag nor env is set", func(t *testing.T) {
		t.Setenv("ATMOS_EDITION", "")
		require.NoError(t, os.Unsetenv("ATMOS_EDITION"))
		assert.Equal(t, "config", EditionPinSource("2026"))
	})
}
