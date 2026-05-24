package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrintAsYAMLWithConfig_SimpleMap(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 2,
				NoColor:  true,
			},
		},
	}
	data := map[string]any{"key": "value", "num": 42}
	err := PrintAsYAMLWithConfig(cfg, data)
	assert.NoError(t, err)
}

func TestPrintAsYAMLWithConfig_NilConfig(t *testing.T) {
	err := PrintAsYAMLWithConfig(nil, map[string]any{"key": "value"})
	assert.ErrorIs(t, err, ErrNilAtmosConfig)
}

func TestPrintAsYAMLWithConfig_NilData(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				NoColor: true,
			},
		},
	}
	err := PrintAsYAMLWithConfig(cfg, nil)
	assert.NoError(t, err)
}

func TestPrintAsYAMLWithConfig_NestedData(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 4,
				NoColor:  true,
			},
		},
	}
	data := map[string]any{
		"environments": map[string]any{
			"dev":  "us-east-1",
			"prod": "us-west-2",
		},
	}
	err := PrintAsYAMLWithConfig(cfg, data)
	assert.NoError(t, err)
}

func TestPrintAsYAMLToFileDescriptor_SimpleMap(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 2,
			},
		},
	}
	data := map[string]any{"region": "us-east-1", "env": "prod"}
	err := PrintAsYAMLToFileDescriptor(cfg, data)
	assert.NoError(t, err)
}

func TestPrintAsYAMLToFileDescriptor_NilConfig(t *testing.T) {
	err := PrintAsYAMLToFileDescriptor(nil, map[string]any{"key": "value"})
	assert.ErrorIs(t, err, ErrNilAtmosConfig)
}

func TestPrintAsYAMLToFileDescriptor_NilData(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	err := PrintAsYAMLToFileDescriptor(cfg, nil)
	require.NoError(t, err)
}
