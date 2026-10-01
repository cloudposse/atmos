//go:build mage

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShardCountFromEnv(t *testing.T) {
	n, err := shardCountFromEnv("10")
	require.NoError(t, err)
	assert.Equal(t, 10, n)

	for _, bad := range []string{"", "0", "-1", "ten"} {
		_, err := shardCountFromEnv(bad)
		require.Error(t, err, bad)
		assert.ErrorIs(t, err, errMissingShardCount)
	}
}
