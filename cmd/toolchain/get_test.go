package toolchain

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Provider and structure tests are in command_provider_test.go.
// This file contains get-specific tests.

func TestGetCommand_Flags(t *testing.T) {
	t.Run("has all flag", func(t *testing.T) {
		flag := getCmd.Flags().Lookup("all")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has limit flag", func(t *testing.T) {
		flag := getCmd.Flags().Lookup("limit")
		require.NotNil(t, flag)
		assert.Equal(t, "10", flag.DefValue)
	})

	t.Run("has format flag", func(t *testing.T) {
		flag := getCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "table", flag.DefValue)
		assert.Equal(t, "f", flag.Shorthand)
	})
}

func TestGetCommand_FlagDescriptions(t *testing.T) {
	tests := []struct {
		flagName string
		contains string
	}{
		{"all", "all"},
		{"limit", "Limit"},
		{"format", "format"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName+" has description", func(t *testing.T) {
			flag := getCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Contains(t, flag.Usage, tt.contains)
		})
	}
}

func TestGetCommand_SupportedFormats(t *testing.T) {
	assert.Equal(t, []string{"table", "plain", "json"}, supportedGetFormats)
}

// TestGetCommand_FormatValidation exercises the real getCmd.RunE (not a re-implementation of its
// logic), so it covers the actual format-validation and plain+all-conflict branches added to
// cmd/toolchain/get.go. Valid formats fall through to toolchain.ListToolVersions with an empty
// tool name, which fails fast on an invalid tool spec without any network or filesystem access
// (pkg/toolchain has its own coverage for ListToolVersions' internal behavior) — what matters
// here is that RunE reaches that call instead of being rejected by one of the two guard checks.
func TestGetCommand_FormatValidation(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		all     string
		wantErr error
	}{
		{name: "table format is valid", format: "table", all: "false"},
		{name: "plain format is valid", format: "plain", all: "false"},
		{name: "json format is valid", format: "json", all: "false"},
		{name: "invalid format is rejected", format: "xml", all: "false", wantErr: errUtils.ErrInvalidFlagValue},
		{name: "empty format is rejected", format: "", all: "false", wantErr: errUtils.ErrInvalidFlagValue},
		{name: "plain with all is rejected", format: "plain", all: "true", wantErr: errUtils.ErrToolchainPlainFormatWithAllFlag},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				require.NoError(t, getCmd.Flags().Set("format", "table"))
				require.NoError(t, getCmd.Flags().Set("all", "false"))
				viper.Reset()
			})

			require.NoError(t, getCmd.Flags().Set("format", tt.format))
			require.NoError(t, getCmd.Flags().Set("all", tt.all))

			err := getCmd.RunE(getCmd, []string{})

			require.Error(t, err)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				// A valid format must reach ListToolVersions rather than being rejected by
				// either guard check — the empty tool name still errors, but with neither
				// guard sentinel.
				assert.NotErrorIs(t, err, errUtils.ErrInvalidFlagValue)
				assert.NotErrorIs(t, err, errUtils.ErrToolchainPlainFormatWithAllFlag)
			}
		})
	}
}

func TestGetCommand_Args(t *testing.T) {
	t.Run("accepts zero arguments", func(t *testing.T) {
		err := getCmd.Args(getCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("accepts one argument", func(t *testing.T) {
		err := getCmd.Args(getCmd, []string{"terraform"})
		assert.NoError(t, err)
	})

	t.Run("rejects two arguments", func(t *testing.T) {
		err := getCmd.Args(getCmd, []string{"terraform", "helm"})
		assert.Error(t, err)
	})
}

func TestGetCommand_DefaultVersionLimit(t *testing.T) {
	assert.Equal(t, 10, defaultVersionLimit)
}
