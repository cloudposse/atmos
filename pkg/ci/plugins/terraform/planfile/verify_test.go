package planfile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func cfgWith(verify schema.PlanfileVerifyMode, storage bool) *schema.AtmosConfiguration {
	pf := schema.PlanfilesConfig{Verify: verify}
	if storage {
		pf.Priority = []string{"github"}
	}
	c := &schema.AtmosConfiguration{}
	c.Components.Terraform.Planfiles = pf
	return c
}

// cfgWithRequired builds a storage-configured config setting both verify and an
// explicit required pointer for the IsPlanRequired tests.
func cfgWithRequired(verify schema.PlanfileVerifyMode, required *bool) *schema.AtmosConfiguration {
	c := cfgWith(verify, true)
	c.Components.Terraform.Planfiles.Required = required
	return c
}

func boolPtr(b bool) *bool { return &b }

func TestResolveVerifyMode(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		ciEnabled   bool
		cliOverride schema.PlanfileVerifyMode
		want        schema.PlanfileVerifyMode
	}{
		// CLI override wins over everything.
		{"cli fail beats config off", cfgWith(schema.PlanfileVerifyOff, true), true, schema.PlanfileVerifyFail, schema.PlanfileVerifyFail},
		{"cli off beats CI default", cfgWith("", true), true, schema.PlanfileVerifyOff, schema.PlanfileVerifyOff},
		{"cli warn beats config fail", cfgWith(schema.PlanfileVerifyFail, true), true, schema.PlanfileVerifyWarn, schema.PlanfileVerifyWarn},

		// Config wins over the CI default.
		{"config warn under CI", cfgWith(schema.PlanfileVerifyWarn, true), true, "", schema.PlanfileVerifyWarn},
		{"config off under CI", cfgWith(schema.PlanfileVerifyOff, true), true, "", schema.PlanfileVerifyOff},
		{"config fail without CI", cfgWith(schema.PlanfileVerifyFail, true), false, "", schema.PlanfileVerifyFail},

		// Default: fail only when CI + storage configured.
		{"default fail under CI with storage", cfgWith("", true), true, "", schema.PlanfileVerifyFail},
		{"default off under CI without storage", cfgWith("", false), true, "", schema.PlanfileVerifyOff},
		{"default off without CI", cfgWith("", true), false, "", schema.PlanfileVerifyOff},

		// Nil config is off.
		{"nil config", nil, true, "", schema.PlanfileVerifyOff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveVerifyMode(tt.atmosConfig, tt.ciEnabled, tt.cliOverride)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPlanRequired(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		ciEnabled   bool
		cliOverride schema.PlanfileVerifyMode
		want        bool
	}{
		// Explicit required wins when verification is active.
		{"explicit true under CI", cfgWithRequired(schema.PlanfileVerifyFail, boolPtr(true)), true, "", true},
		{"explicit false while verify fails", cfgWithRequired(schema.PlanfileVerifyFail, boolPtr(false)), true, "", false},
		{"explicit true while verify warns", cfgWithRequired(schema.PlanfileVerifyWarn, boolPtr(true)), true, "", true},

		// verify=off short-circuits to not-required, even with required:true.
		{"verify off short-circuits explicit true", cfgWithRequired(schema.PlanfileVerifyOff, boolPtr(true)), true, "", false},
		{"no-verify-plan CLI override beats required true", cfgWithRequired(schema.PlanfileVerifyFail, boolPtr(true)), true, schema.PlanfileVerifyOff, false},

		// Unset: tracks verify strictness (required only when verify resolves to fail).
		{"unset tracks default fail under CI with storage", cfgWith("", true), true, "", true},
		{"unset not required under warn", cfgWith(schema.PlanfileVerifyWarn, true), true, "", false},
		{"unset not required under off", cfgWith(schema.PlanfileVerifyOff, true), true, "", false},

		// Negative path: never require a stored plan on a local (non-CI) deploy by default.
		{"unset not required without CI", cfgWith("", true), false, "", false},
		{"unset not required under CI without storage", cfgWith("", false), true, "", false},

		// Nil config is not required.
		{"nil config", nil, true, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPlanRequired(tt.atmosConfig, tt.ciEnabled, tt.cliOverride)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStorageConfigured(t *testing.T) {
	assert.False(t, StorageConfigured(&schema.PlanfilesConfig{}))
	assert.True(t, StorageConfigured(&schema.PlanfilesConfig{Default: "github"}))
	assert.True(t, StorageConfigured(&schema.PlanfilesConfig{Priority: []string{"s3"}}))
	assert.True(t, StorageConfigured(&schema.PlanfilesConfig{Stores: map[string]schema.PlanfileStoreSpec{"x": {}}}))
}
