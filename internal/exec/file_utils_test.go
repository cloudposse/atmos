package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPrintOrWriteToFile(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 4,
			},
		},
	}

	// Test data to write
	testData := map[string]interface{}{
		"test": "data",
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	err := printOrWriteToFile(atmosConfig, "yaml", "", testData)
	assert.NoError(t, err)

	err = printOrWriteToFile(atmosConfig, "json", "", testData)
	assert.NoError(t, err)

	tempDir, err := os.MkdirTemp("", "atmos-test-*")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	yamlFile := filepath.Join(tempDir, "test.yaml")
	err = printOrWriteToFile(atmosConfig, "yaml", yamlFile, testData)
	assert.NoError(t, err)

	_, err = os.Stat(yamlFile)
	assert.NoError(t, err)

	jsonFile := filepath.Join(tempDir, "test.json")
	err = printOrWriteToFile(atmosConfig, "json", jsonFile, testData)
	assert.NoError(t, err)

	_, err = os.Stat(jsonFile)
	assert.NoError(t, err)

	err = printOrWriteToFile(atmosConfig, "invalid", "", testData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid 'format'")

	// Test with default tab width (when TabWidth is 0)
	atmosConfigDefaultTabWidth := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 0, // Should default to 2
			},
		},
	}
	err = printOrWriteToFile(atmosConfigDefaultTabWidth, "yaml", "", testData)
	assert.NoError(t, err)
}
