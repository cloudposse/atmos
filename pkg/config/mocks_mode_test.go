package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: these tests reference specific schema fields.
var (
	_ = schema.TerraformMocks{Mode: schema.TerraformMocksModeFallback}
	_ = schema.ConfigAndStacksInfo{MocksMode: "always"}
)

func TestTerraformMocksModeIsValid(t *testing.T) {
	tests := []struct {
		mode schema.TerraformMocksMode
		want bool
	}{
		{mode: "", want: true},
		{mode: schema.TerraformMocksModeFallback, want: true},
		{mode: schema.TerraformMocksModeAlways, want: true},
		{mode: "never", want: false},
		{mode: "Fallback", want: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.mode.IsValid())
		})
	}
}

func TestEffectiveMocksMode(t *testing.T) {
	assert.Equal(t, schema.TerraformMocksModeFallback, (&schema.Terraform{}).EffectiveMocksMode())
	assert.Equal(t, schema.TerraformMocksModeAlways, (&schema.Terraform{Mocks: schema.TerraformMocks{Mode: schema.TerraformMocksModeAlways}}).EffectiveMocksMode())
}

// TestLoadConfigEditionMocksMode covers components.terraform.mocks.mode's journal entry (dated
// 2026-10-01): a brand-new key that governs behavior --use-mocks already had a fixed answer for
// (always), so a project pinned before the date keeps the hermetic behavior.
func TestLoadConfigEditionMocksMode(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want schema.TerraformMocksMode
	}{
		{name: "no pin gets the fallback default", yaml: "base_path: ./\n", want: schema.TerraformMocksModeFallback},
		{name: "pin on the release date gets fallback", yaml: "base_path: ./\nedition: \"2026-10-01\"\n", want: schema.TerraformMocksModeFallback},
		{name: "pin one day before the release date restores always", yaml: "base_path: ./\nedition: \"2026-09-30\"\n", want: schema.TerraformMocksModeAlways},
		{name: "an old pin restores always", yaml: "base_path: ./\nedition: \"2026-01\"\n", want: schema.TerraformMocksModeAlways},
		{
			name: "explicit user value beats the pin",
			yaml: "base_path: ./\nedition: \"2026-01\"\ncomponents:\n  terraform:\n    mocks:\n      mode: fallback\n",
			want: schema.TerraformMocksModeFallback,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeEditionTestConfig(t, tt.yaml)

			atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)

			assert.Equal(t, tt.want, atmosConfig.Components.Terraform.Mocks.Mode)
		})
	}
}

func TestMocksModeEnvVarAndFlagPrecedence(t *testing.T) {
	tests := []struct {
		name     string
		start    schema.TerraformMocksMode // value loaded from config/defaults.
		env      string
		flag     string
		want     schema.TerraformMocksMode
		wantErrs error
	}{
		{name: "config value is kept without env or flag", start: schema.TerraformMocksModeAlways, want: schema.TerraformMocksModeAlways},
		{name: "env beats config", start: schema.TerraformMocksModeFallback, env: "always", want: schema.TerraformMocksModeAlways},
		{name: "env is trimmed and lower-cased", start: schema.TerraformMocksModeAlways, env: " Fallback ", want: schema.TerraformMocksModeFallback},
		{name: "flag beats config", start: schema.TerraformMocksModeFallback, flag: "always", want: schema.TerraformMocksModeAlways},
		{name: "flag beats env", start: schema.TerraformMocksModeFallback, env: "always", flag: "fallback", want: schema.TerraformMocksModeFallback},
		{name: "invalid env errors", env: "sometimes", wantErrs: errUtils.ErrInvalidMocksMode},
		{name: "invalid flag errors", flag: "sometimes", wantErrs: errUtils.ErrInvalidMocksMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_COMPONENTS_TERRAFORM_MOCKS_MODE", tt.env)
			atmosConfig := &schema.AtmosConfiguration{Schemas: make(map[string]interface{})}
			atmosConfig.Components.Terraform.Mocks.Mode = tt.start

			err := processEnvVars(atmosConfig)
			if err == nil {
				err = setFeatureFlags(atmosConfig, &schema.ConfigAndStacksInfo{MocksMode: tt.flag})
			}

			if tt.wantErrs != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErrs))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, atmosConfig.Components.Terraform.Mocks.Mode)
		})
	}
}

func TestParseUseMocksFlag(t *testing.T) {
	tests := []struct {
		raw         string
		wantEnabled bool
		wantMode    string
		wantErr     bool
	}{
		{raw: "", wantEnabled: false},
		{raw: "false", wantEnabled: false},
		{raw: "FALSE", wantEnabled: false},
		{raw: "true", wantEnabled: true},
		{raw: " True ", wantEnabled: true},
		{raw: "fallback", wantEnabled: true, wantMode: "fallback"},
		{raw: "always", wantEnabled: true, wantMode: "always"},
		{raw: "Always", wantEnabled: true, wantMode: "always"},
		{raw: "sometimes", wantErr: true},
		{raw: "1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			enabled, mode, err := ParseUseMocksFlag(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrInvalidFlagValue))
				assert.False(t, enabled)
				assert.Empty(t, mode)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, enabled)
			assert.Equal(t, tt.wantMode, mode)
		})
	}
}
