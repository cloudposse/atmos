package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: these tests reference the toolchain.install field.
var _ = schema.Toolchain{Install: schema.ToolchainInstallAuto}

// TestLoadConfigEditionToolchainInstall covers toolchain.install's journal entry (dated
// 2026-10-09): a brand-new key that governs behavior Atmos already had a fixed answer for
// (only explicit dependencies were installed), so a project pinned before the date keeps it.
func TestLoadConfigEditionToolchainInstall(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		env  string
		want schema.ToolchainInstall
	}{
		{name: "no pin gets the auto default", yaml: "base_path: ./\n", want: schema.ToolchainInstallAuto},
		{name: "pin on the release date gets auto", yaml: "base_path: ./\nedition: \"2026-10-09\"\n", want: schema.ToolchainInstallAuto},
		{name: "pin one day before the release date restores declared", yaml: "base_path: ./\nedition: \"2026-10-08\"\n", want: schema.ToolchainInstallDeclared},
		{name: "an old pin restores declared", yaml: "base_path: ./\nedition: \"2026-01\"\n", want: schema.ToolchainInstallDeclared},
		{name: "a year pin before the release restores declared", yaml: "base_path: ./\nedition: \"2025\"\n", want: schema.ToolchainInstallDeclared},
		{name: "a year pin that includes the release gets auto", yaml: "base_path: ./\nedition: \"2026\"\n", want: schema.ToolchainInstallAuto},
		{
			name: "explicit user value beats the pin",
			yaml: "base_path: ./\nedition: \"2026-01\"\ntoolchain:\n  install: always\n",
			want: schema.ToolchainInstallAlways,
		},
		{
			name: "environment variable beats the pin",
			yaml: "base_path: ./\nedition: \"2026-01\"\n",
			env:  "never",
			want: schema.ToolchainInstallNever,
		},
		{
			name: "environment variable beats the config file",
			yaml: "base_path: ./\ntoolchain:\n  install: always\n",
			env:  "never",
			want: schema.ToolchainInstallNever,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeEditionTestConfig(t, tt.yaml)
			t.Setenv("ATMOS_TOOLCHAIN_INSTALL", tt.env)

			atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)

			assert.Equal(t, tt.want, atmosConfig.Toolchain.Install)
		})
	}
}

func TestToolchainInstallEnvVarDoesNotShadowInstallPath(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\ntoolchain:\n  install_path: custom-tools\n")
	t.Setenv("ATMOS_TOOLCHAIN_INSTALL", "never")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)

	assert.Equal(t, schema.ToolchainInstallNever, atmosConfig.Toolchain.Install)
	assert.Equal(t, "custom-tools", atmosConfig.Toolchain.InstallPath)
}

func TestNormalizeConfiguredToolchainInstall(t *testing.T) {
	tests := []struct {
		name    string
		value   schema.ToolchainInstall
		want    schema.ToolchainInstall
		wantErr bool
	}{
		{name: "empty stays empty", value: "", want: ""},
		{name: "never", value: "never", want: schema.ToolchainInstallNever},
		{name: "declared", value: "declared", want: schema.ToolchainInstallDeclared},
		{name: "auto", value: "auto", want: schema.ToolchainInstallAuto},
		{name: "always", value: "always", want: schema.ToolchainInstallAlways},
		{name: "lower-cased and trimmed", value: " Always ", want: schema.ToolchainInstallAlways},
		{name: "unknown value", value: "sometimes", wantErr: true},
		{name: "boolean-looking value", value: "true", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{Schemas: make(map[string]interface{})}
			atmosConfig.Toolchain.Install = tt.value

			err := processEnvVars(atmosConfig)

			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrInvalidToolchainInstall)
				formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
				assert.Contains(t, formatted, string(tt.value))
				for _, valid := range []string{"never", "declared", "auto", "always"} {
					assert.Contains(t, formatted, valid)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, atmosConfig.Toolchain.Install)
		})
	}
}

func TestInvalidToolchainInstallEnvVarFailsAtConfigLoad(t *testing.T) {
	writeEditionTestConfig(t, "base_path: ./\n")
	t.Setenv("ATMOS_TOOLCHAIN_INSTALL", "sometimes")

	atmosConfig, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	require.ErrorIs(t, processEnvVars(&atmosConfig), errUtils.ErrInvalidToolchainInstall)
}
