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

// cfgWithMissing builds a storage-configured config setting both verify and
// on_missing for the missing-plan resolution tests.
func cfgWithMissing(verify, onMissing schema.PlanfileVerifyMode) *schema.AtmosConfiguration {
	c := cfgWith(verify, true)
	c.Components.Terraform.Planfiles.OnMissing = onMissing
	return c
}

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

func TestResolveMissingMode(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		ciEnabled   bool
		cliOverride schema.PlanfileVerifyMode
		want        schema.PlanfileVerifyMode
	}{
		// CLI override wins over everything.
		{"cli fail beats config off", cfgWithMissing(schema.PlanfileVerifyOff, schema.PlanfileVerifyOff), true, schema.PlanfileVerifyFail, schema.PlanfileVerifyFail},

		// Explicit on_missing config wins over the verify-tracking default.
		{"on_missing off while verify fails", cfgWithMissing(schema.PlanfileVerifyFail, schema.PlanfileVerifyOff), true, "", schema.PlanfileVerifyOff},
		{"on_missing warn while verify fails", cfgWithMissing(schema.PlanfileVerifyFail, schema.PlanfileVerifyWarn), true, "", schema.PlanfileVerifyWarn},
		{"on_missing fail while verify off", cfgWithMissing(schema.PlanfileVerifyOff, schema.PlanfileVerifyFail), true, "", schema.PlanfileVerifyFail},

		// Unset on_missing tracks the resolved verify mode.
		{"tracks default fail under CI with storage", cfgWith("", true), true, "", schema.PlanfileVerifyFail},
		{"tracks config warn", cfgWith(schema.PlanfileVerifyWarn, true), true, "", schema.PlanfileVerifyWarn},
		{"tracks config off", cfgWith(schema.PlanfileVerifyOff, true), true, "", schema.PlanfileVerifyOff},

		// Negative path: never fail a local (non-CI) deploy by default.
		{"off without CI even with storage", cfgWith("", true), false, "", schema.PlanfileVerifyOff},
		{"off under CI without storage", cfgWith("", false), true, "", schema.PlanfileVerifyOff},

		// Nil config is off.
		{"nil config", nil, true, "", schema.PlanfileVerifyOff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveMissingMode(tt.atmosConfig, tt.ciEnabled, tt.cliOverride)
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
