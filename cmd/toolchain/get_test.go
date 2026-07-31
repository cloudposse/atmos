package toolchain

import (
	"bytes"
	"slices"
	"testing"

	"github.com/spf13/cobra"
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

func TestGetCommand_FormatValidation(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		all     bool
		wantErr error
	}{
		{name: "table format is valid", format: "table"},
		{name: "plain format is valid", format: "plain"},
		{name: "json format is valid", format: "json"},
		{name: "invalid format is rejected", format: "xml", wantErr: errUtils.ErrInvalidFlagValue},
		{name: "empty format is rejected", format: "", wantErr: errUtils.ErrInvalidFlagValue},
		{name: "plain with all is rejected", format: "plain", all: true, wantErr: errUtils.ErrToolchainPlainFormatWithAllFlag},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("format", tt.format)
			v.Set("all", tt.all)

			// Mimic getCmd's RunE validation logic without invoking network/filesystem lookups.
			testCmd := &cobra.Command{
				Use: "get",
				RunE: func(cmd *cobra.Command, args []string) error {
					format := v.GetString("format")
					all := v.GetBool("all")
					if !slices.Contains(supportedGetFormats, format) {
						return errUtils.ErrInvalidFlagValue
					}
					if format == "plain" && all {
						return errUtils.ErrToolchainPlainFormatWithAllFlag
					}
					return nil
				},
			}

			var stdout, stderr bytes.Buffer
			testCmd.SetOut(&stdout)
			testCmd.SetErr(&stderr)

			err := testCmd.Execute()

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
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
