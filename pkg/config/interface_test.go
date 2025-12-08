package config

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDefaultLoader_InitCliConfig verifies that DefaultLoader correctly delegates to InitCliConfig.
func TestDefaultLoader_InitCliConfig(t *testing.T) {
	loader := &DefaultLoader{}

	// Create minimal ConfigAndStacksInfo.
	configInfo := &schema.ConfigAndStacksInfo{}

	// Call InitCliConfig with processStacks=false to avoid needing real stack files.
	_, err := loader.InitCliConfig(configInfo, false)

	// We expect this to succeed.
	// BasePath may be empty if no atmos.yaml exists, which is acceptable.
	assert.NoError(t, err)
}
