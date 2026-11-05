package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWriteToFileAsYAMLWithConfigPreservesStringScalars(t *testing.T) {
	for _, value := range []string{"true", "false", "null", "123", "1.5", "", "key: value", "line one\nline two"} {
		t.Run(value, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "value.yaml")
			require.NoError(t, WriteToFileAsYAMLWithConfig(&schema.AtmosConfiguration{}, path, value, 0o600))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var decoded any
			require.NoError(t, yaml.Unmarshal(data, &decoded))
			assert.Equal(t, value, decoded)
		})
	}
}
