package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestroyCmd_MultiComponentFlagsRegistered(t *testing.T) {
	require.NotNil(t, destroyParser)
	assert.NotNil(t, destroyCmd.Flags().Lookup("all"), "destroy should register --all")
	assert.NotNil(t, destroyCmd.Flags().Lookup("affected"), "destroy should register --affected")
	assert.NotNil(t, destroyCmd.Flags().Lookup("auto-generate-backend-file"), "destroy should register backend execution flags")
}
