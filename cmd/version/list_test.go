package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListCommand_Flags(t *testing.T) {
	// Test that list command has required flags.
	limitFlag := listCmd.Flags().Lookup("limit")
	assert.NotNil(t, limitFlag)
	assert.Equal(t, "10", limitFlag.DefValue)

	offsetFlag := listCmd.Flags().Lookup("offset")
	assert.NotNil(t, offsetFlag)
	assert.Equal(t, "0", offsetFlag.DefValue)

	sinceFlag := listCmd.Flags().Lookup("since")
	assert.NotNil(t, sinceFlag)

	includePrereleasesFlag := listCmd.Flags().Lookup("include-prereleases")
	assert.NotNil(t, includePrereleasesFlag)
	assert.Equal(t, "false", includePrereleasesFlag.DefValue)

	formatFlag := listCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag)
	assert.Equal(t, "text", formatFlag.DefValue)
}

func TestListCommand_BasicProperties(t *testing.T) {
	assert.Equal(t, "list", listCmd.Use)
	assert.NotEmpty(t, listCmd.Short)
	assert.NotEmpty(t, listCmd.Long)
	assert.NotEmpty(t, listCmd.Example)
	assert.NotNil(t, listCmd.RunE)
}
