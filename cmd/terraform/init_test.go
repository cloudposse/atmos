package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitBulkExecutionFlagsRegistered(t *testing.T) {
	for _, name := range []string{"all", "affected", "max-concurrency", "failure-mode", "log-order"} {
		f := initCmd.Flags().Lookup(name)
		require.NotNil(t, f, "init must register --%s", name)
	}

	assert.Equal(t, "false", initCmd.Flags().Lookup("all").DefValue)
	assert.Equal(t, "false", initCmd.Flags().Lookup("affected").DefValue)
	assert.Equal(t, "1", initCmd.Flags().Lookup("max-concurrency").DefValue)
	assert.Equal(t, "fail-fast", initCmd.Flags().Lookup("failure-mode").DefValue)
	assert.Equal(t, "stream", initCmd.Flags().Lookup("log-order").DefValue)
}
