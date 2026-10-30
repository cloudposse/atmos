package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// The legacy init_run_reconfigure env var and --init-run-reconfigure flag must keep overriding
// configuration -- including an explicit init.reconfigure (e.g. one written by `atmos doctor --fix`)
// -- so CLI > ENV > config precedence holds. The newer init.reconfigure env var / flag still win over
// their legacy counterparts at the same level.

const (
	envLegacyReconfigure = "ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE"
	envNewReconfigure    = "ATMOS_COMPONENTS_TERRAFORM_INIT_RECONFIGURE"
)

func TestInitReconfigurePrecedence_LegacyEnvOverridesConfig(t *testing.T) {
	tests := []struct {
		name          string
		configured    schema.TerraformInitReconfigure
		legacyEnv     string
		newEnv        string
		wantEffective schema.TerraformInitReconfigure
	}{
		{name: "legacy false overrides explicit auto", configured: schema.TerraformInitReconfigureAuto, legacyEnv: "false", wantEffective: schema.TerraformInitReconfigureNever},
		{name: "legacy false overrides explicit always", configured: schema.TerraformInitReconfigureAlways, legacyEnv: "false", wantEffective: schema.TerraformInitReconfigureNever},
		{name: "legacy true overrides explicit never", configured: schema.TerraformInitReconfigureNever, legacyEnv: "true", wantEffective: schema.TerraformInitReconfigureAuto},
		{name: "legacy false with nothing configured", legacyEnv: "false", wantEffective: schema.TerraformInitReconfigureNever},
		{name: "newer env var wins over legacy env var", configured: schema.TerraformInitReconfigureNever, legacyEnv: "false", newEnv: "always", wantEffective: schema.TerraformInitReconfigureAlways},
		{name: "no env leaves explicit config alone", configured: schema.TerraformInitReconfigureAlways, wantEffective: schema.TerraformInitReconfigureAlways},
		{name: "newer env var alone overrides config", configured: schema.TerraformInitReconfigureAlways, newEnv: "never", wantEffective: schema.TerraformInitReconfigureNever},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envLegacyReconfigure, tt.legacyEnv)
			t.Setenv(envNewReconfigure, tt.newEnv)

			cfg := schema.AtmosConfiguration{}
			cfg.Components.Terraform.InitRunReconfigure = true
			cfg.Components.Terraform.Init.Reconfigure = tt.configured
			require.NoError(t, processEnvVars(&cfg))

			assert.Equal(t, tt.wantEffective, cfg.Components.Terraform.EffectiveInitReconfigure())
		})
	}
}

func TestInitReconfigurePrecedence_LegacyFlagOverridesConfigAndEnv(t *testing.T) {
	tests := []struct {
		name          string
		configured    schema.TerraformInitReconfigure
		newEnv        string
		legacyFlag    string
		newFlag       string
		wantEffective schema.TerraformInitReconfigure
	}{
		{name: "legacy flag false overrides explicit auto", configured: schema.TerraformInitReconfigureAuto, legacyFlag: "false", wantEffective: schema.TerraformInitReconfigureNever},
		{name: "legacy flag true overrides explicit never", configured: schema.TerraformInitReconfigureNever, legacyFlag: "true", wantEffective: schema.TerraformInitReconfigureAuto},
		{name: "legacy flag beats newer env var (CLI > ENV)", newEnv: "always", legacyFlag: "false", wantEffective: schema.TerraformInitReconfigureNever},
		{name: "newer flag wins over legacy flag", legacyFlag: "false", newFlag: "always", wantEffective: schema.TerraformInitReconfigureAlways},
		{name: "no legacy flag leaves explicit config alone", configured: schema.TerraformInitReconfigureAlways, wantEffective: schema.TerraformInitReconfigureAlways},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envLegacyReconfigure, "")
			t.Setenv(envNewReconfigure, tt.newEnv)

			cfg := schema.AtmosConfiguration{}
			cfg.Components.Terraform.InitRunReconfigure = true
			cfg.Components.Terraform.Init.Reconfigure = tt.configured
			require.NoError(t, processEnvVars(&cfg))

			info := schema.ConfigAndStacksInfo{InitRunReconfigure: tt.legacyFlag, InitReconfigure: tt.newFlag}
			require.NoError(t, setFeatureFlags(&cfg, &info))

			assert.Equal(t, tt.wantEffective, cfg.Components.Terraform.EffectiveInitReconfigure())
		})
	}
}

// With no atmos.yaml the built-in defaults leave init.reconfigure empty; the legacy overrides must still work.
func TestInitReconfigurePrecedence_DefaultConfigStillHonorsLegacy(t *testing.T) {
	t.Setenv(envLegacyReconfigure, "false")
	t.Setenv(envNewReconfigure, "")

	cfg := defaultCliConfig
	require.Empty(t, cfg.Components.Terraform.Init.Reconfigure)
	require.NoError(t, processEnvVars(&cfg))
	assert.Equal(t, schema.TerraformInitReconfigureNever, cfg.Components.Terraform.EffectiveInitReconfigure())
}
