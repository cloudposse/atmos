package version

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCommandProvider(t *testing.T) {
	provider := &VersionCommandProvider{}

	assert.Equal(t, "version", provider.GetName())
	assert.Equal(t, "Other Commands", provider.GetGroup())
	assert.NotNil(t, provider.GetCommand())
}

func TestVersionCommand_Flags(t *testing.T) {
	cmd := versionCmd

	checkFlagLookup := cmd.Flags().Lookup("check")
	assert.NotNil(t, checkFlagLookup)
	assert.Equal(t, "c", checkFlagLookup.Shorthand)

	formatFlagLookup := cmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlagLookup)

	shortFlagLookup := cmd.Flags().Lookup("short")
	assert.NotNil(t, shortFlagLookup)
	assert.Equal(t, "s", shortFlagLookup.Shorthand)
	assert.Equal(t, "false", shortFlagLookup.DefValue)
}

func TestVersionCommand_BasicProperties(t *testing.T) {
	cmd := versionCmd

	assert.Equal(t, "version", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Equal(t, "atmos version", cmd.Example)
	assert.NotNil(t, cmd.RunE)
}

func TestSetAtmosConfig(t *testing.T) {
	// Test that SetAtmosConfig accepts a nil pointer without panic.
	SetAtmosConfig(nil)

	// SetAtmosConfig should complete without error.
	// Note: We can't easily test the side effect without refactoring,
	// but we verify it doesn't panic.
	assert.NotPanics(t, func() {
		SetAtmosConfig(nil)
	})
}

func TestParseVersionOptions_ShortFlag(t *testing.T) {
	tests := []struct {
		name           string
		short          bool
		format         string
		expectedFormat string
	}{
		{
			name:           "short flag sets plain format when no format specified",
			short:          true,
			format:         "",
			expectedFormat: "plain",
		},
		{
			name:           "short flag does not override explicit format",
			short:          true,
			format:         "json",
			expectedFormat: "json",
		},
		{
			name:           "no short flag preserves empty format",
			short:          false,
			format:         "",
			expectedFormat: "",
		},
		{
			name:           "no short flag preserves explicit format",
			short:          false,
			format:         "yaml",
			expectedFormat: "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("short", tt.short)
			v.Set("format", tt.format)
			v.Set("check", false)

			opts, err := parseVersionOptions(versionCmd, v, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedFormat, opts.Format)
			assert.Equal(t, tt.short, opts.Short)
		})
	}
}
