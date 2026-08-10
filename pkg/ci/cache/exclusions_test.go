package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultExcludedPaths_ContainsKnownAuthSubdirs(t *testing.T) {
	assert.Equal(t, []string{"aws-sso", "azure-device-code", "aws-webflow", "auth"}, DefaultExcludedPaths())
}

func TestDefaultExcludedPaths_ReturnsCopy(t *testing.T) {
	got := DefaultExcludedPaths()
	got[0] = "mutated"

	again := DefaultExcludedPaths()
	assert.Equal(t, "aws-sso", again[0], "mutating the returned slice must not affect the source list")
}
