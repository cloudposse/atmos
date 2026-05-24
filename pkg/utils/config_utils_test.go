package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractAtmosConfig_DirectValue(t *testing.T) {
	cfg := schema.AtmosConfiguration{
		BasePath: "/test/path",
	}
	result := ExtractAtmosConfig(cfg)
	assert.Equal(t, "/test/path", result.BasePath)
}

func TestExtractAtmosConfig_Pointer(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		BasePath: "/another/path",
	}
	result := ExtractAtmosConfig(cfg)
	assert.Equal(t, "/another/path", result.BasePath)
}

func TestExtractAtmosConfig_Unknown(t *testing.T) {
	// Unknown type should return empty config.
	result := ExtractAtmosConfig("not a config")
	assert.Equal(t, schema.AtmosConfiguration{}, result)
}

func TestExtractAtmosConfig_Nil(t *testing.T) {
	// Nil should return empty config.
	result := ExtractAtmosConfig(nil)
	assert.Equal(t, schema.AtmosConfiguration{}, result)
}

func TestExtractAtmosConfig_Int(t *testing.T) {
	// Integer type returns empty config.
	result := ExtractAtmosConfig(42)
	assert.Equal(t, schema.AtmosConfiguration{}, result)
}
