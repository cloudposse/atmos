package configuration

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: these tests depend on the recorded unknown keys.
var _ = schema.Terraform{UnknownSections: []string{"guard"}}

// Pins on each side of the transition. The day before the journal date keeps the old behavior.
const (
	legacyPin = "2026-11-26"
	strictPin = "2026-11-27"
)

// captureWarnings records warnings logged during one test and forgets the sections already warned
// about, so each test sees its own warnings.
func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	level := log.GetLevel()
	log.SetOutput(&output)
	log.SetLevel(log.WarnLevel)
	warnedSections.Clear()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(level)
		warnedSections.Clear()
	})
	return &output
}

func noExports(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
	return map[string]bool{}, true, nil
}

func pinnedConfig(pin string, sections map[string]schema.ComponentConfiguration, unknown ...string) *schema.AtmosConfiguration {
	config := sectionsConfig(sections)
	config.Edition = pin
	config.Components.Terraform.UnknownSections = unknown
	return config
}

func TestUnknownSectionsTransitionIsJournaled(t *testing.T) {
	require.Equal(t, strictPin, transitionDate(), "the pins in these tests straddle the journal date")
	before, err := edition.BehaviorChanged(legacyPin, UnknownSectionsBehavior)
	require.NoError(t, err)
	require.False(t, before)
	on, err := edition.BehaviorChanged(strictPin, UnknownSectionsBehavior)
	require.NoError(t, err)
	require.True(t, on)
}

// The issue's example: a project's own mapping with `enabled` under components.terraform.
func TestTeamDefaultsSectionByEdition(t *testing.T) {
	section := map[string]schema.ComponentConfiguration{
		"team_defaults": {Enabled: true, Options: map[string]any{"owner": "platform"}},
	}
	for _, pin := range []string{"", strictPin, "2027"} {
		t.Run("rejected/"+pin, func(t *testing.T) {
			output := captureWarnings(t)
			useCatalog(t, noExports)
			err := selectProviders(t.Context(), pinnedConfig(pin, section))
			require.ErrorIs(t, err, errUtils.ErrUnknownTerraformSection)
			require.Contains(t, cockroach.FlattenDetails(err), "`components.terraform.team_defaults`")
			require.Empty(t, output.String(), "the error replaces the warning")
		})
	}
	for _, pin := range []string{legacyPin, "2026-10"} {
		t.Run("ignored/"+pin, func(t *testing.T) {
			output := captureWarnings(t)
			useCatalog(t, noExports)
			config := pinnedConfig(pin, section)
			require.NoError(t, selectProviders(t.Context(), config))
			require.Empty(t, config.Components.Terraform.ConfigurationProviders, "the section is still dropped")
			require.Contains(t, output.String(), "components.terraform.team_defaults")
		})
	}
}

// A misspelled setting never reaches ConfigurationProviders; the loader records its name instead.
func TestMisspelledSettingByEdition(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		captureWarnings(t)
		useCatalog(t, noExports)
		err := selectProviders(t.Context(), pinnedConfig("", nil, "base_pth"))
		require.ErrorIs(t, err, errUtils.ErrUnknownTerraformSection)
		hints := strings.Join(cockroach.GetAllHints(err), "\n")
		require.Contains(t, hints, "misspelled setting")
		require.Contains(t, hints, "plugins.packages")
		require.Contains(t, hints, "remove it")
		require.Contains(t, hints, "pin `edition` to a date before "+strictPin)
		require.Contains(t, cockroach.FlattenDetails(err), "`components.terraform.base_pth`")
	})
	t.Run("ignored once per section", func(t *testing.T) {
		output := captureWarnings(t)
		useCatalog(t, noExports)
		require.NoError(t, selectProviders(t.Context(), pinnedConfig(legacyPin, nil, "base_pth")))
		require.NoError(t, selectProviders(t.Context(), pinnedConfig(legacyPin, nil, "base_pth")))
		require.Equal(t, 1, strings.Count(output.String(), "components.terraform.base_pth"))
	})
}

func TestUnclaimedSectionsAreListedTogether(t *testing.T) {
	captureWarnings(t)
	useCatalog(t, noExports)
	config := pinnedConfig("", map[string]schema.ComponentConfiguration{
		"team_defaults": {Enabled: true},
		"terragrunt":    {Enabled: true},
	}, "zeta", "alpha")
	err := selectProviders(t.Context(), config)
	require.ErrorIs(t, err, errUtils.ErrUnknownTerraformSection)
	details := cockroach.FlattenDetails(err)
	require.Contains(t, details, "`components.terraform.alpha`, `components.terraform.team_defaults`, `components.terraform.zeta`")
	require.NotContains(t, details, "components.terraform.terragrunt")
}

// Negative paths: claimed sections are valid under every edition.
func TestClaimedSectionsAreNeverReported(t *testing.T) {
	for _, pin := range []string{"", legacyPin} {
		t.Run(pin, func(t *testing.T) {
			output := captureWarnings(t)
			useCatalog(t, func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
				return map[string]bool{"custom-plugin": true, "hcl": true}, true, nil
			})
			config := pinnedConfig(pin, map[string]schema.ComponentConfiguration{
				"terragrunt": {Enabled: true},
				"tg":         {Enabled: true, Provider: "terragrunt"},
				"custom":     {Enabled: true, Provider: "custom-plugin"},
			}, "hcl", "terragrunt")
			require.NoError(t, selectProviders(t.Context(), config))
			require.Len(t, config.Components.Terraform.ConfigurationProviders, 3)
			require.Empty(t, output.String())
		})
	}
}

// When the exports of an enabled package are unknown, nothing is reported: the package might claim
// the section, and its provider reports the problem once installed.
func TestUnknownExportsReportNothing(t *testing.T) {
	for name, catalog := range map[string]ProviderCatalog{
		"error": func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
			return nil, false, errors.New("lock file unreadable")
		},
		"incomplete": func(context.Context, *schema.AtmosConfiguration) (map[string]bool, bool, error) {
			return map[string]bool{}, false, nil
		},
		"unregistered": nil,
	} {
		t.Run(name, func(t *testing.T) {
			output := captureWarnings(t)
			useCatalog(t, catalog)
			config := pinnedConfig("", map[string]schema.ComponentConfiguration{"tool": {Enabled: true}}, "base_pth")
			require.NoError(t, selectProviders(t.Context(), config))
			require.Contains(t, config.Components.Terraform.ConfigurationProviders, "tool")
			require.Empty(t, output.String())
		})
	}
}

func TestInvalidEditionPinIsReported(t *testing.T) {
	captureWarnings(t)
	useCatalog(t, noExports)
	err := selectProviders(t.Context(), pinnedConfig("not-a-date", nil, "base_pth"))
	require.Error(t, err)
	require.NotErrorIs(t, err, errUtils.ErrUnknownTerraformSection)
}

func TestAttachRejectsUnknownSectionBeforeStartingAProvider(t *testing.T) {
	captureWarnings(t)
	useCatalog(t, noExports)
	factories.Lock()
	previous := factories.values
	factories.values = map[string]Factory{"plugin": func(context.Context, *schema.AtmosConfiguration, string) (schema.ComponentConfigurationRuntime, error) {
		t.Fatal("an unknown section must not start a provider session")
		return nil, nil
	}}
	factories.Unlock()
	t.Cleanup(func() { factories.Lock(); factories.values = previous; factories.Unlock() })
	config := pinnedConfig("", map[string]schema.ComponentConfiguration{"team_defaults": {Enabled: true}})
	require.ErrorIs(t, Attach(t.Context(), config, "list"), errUtils.ErrUnknownTerraformSection)
	require.Nil(t, config.ComponentConfigurations)
}
