package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
}

func TestVersionCommand_BasicProperties(t *testing.T) {
	cmd := versionCmd

	assert.Equal(t, "version", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.Equal(t, "atmos version", cmd.Example)
	assert.NotNil(t, cmd.RunE)
}
