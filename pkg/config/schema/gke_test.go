package configschema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeneratedSchemaIncludesGKEClusterFields verifies generated config schema coverage.
func TestGeneratedSchemaIncludesGKEClusterFields(t *testing.T) {
	cluster, ok := definitions(t)["Cluster"].(map[string]any)
	require.True(t, ok)
	properties, ok := cluster["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, properties, "project_id")
	assert.Contains(t, properties, "location")
}
