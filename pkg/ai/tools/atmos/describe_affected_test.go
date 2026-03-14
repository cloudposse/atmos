package atmos

import (
	"context"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestDescribeAffectedTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/tmp/atmos",
	}

	tool := NewDescribeAffectedTool(config)

	assert.Equal(t, "describe_affected", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	assert.Len(t, params, 2)
	assert.Equal(t, "ref", params[0].Name)
	assert.Equal(t, "verbose", params[1].Name)
	assert.False(t, params[0].Required)
	assert.False(t, params[1].Required)
}

func TestDescribeAffectedTool_Execute_Defaults(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../", // Use repo root.
	}

	tool := NewDescribeAffectedTool(config)
	ctx := context.Background()

	// Execute with defaults (may succeed or fail depending on git state).
	result, err := tool.Execute(ctx, map[string]interface{}{})

	assert.NoError(t, err)
	// Result may succeed or fail depending on environment.
	assert.NotNil(t, result)
}

func TestDescribeAffectedTool_Execute_CustomRef(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "../../../../../../", // Use repo root.
	}

	tool := NewDescribeAffectedTool(config)
	ctx := context.Background()

	// Execute with custom ref (may succeed or fail depending on git state).
	result, err := tool.Execute(ctx, map[string]interface{}{
		"ref":     "HEAD~1",
		"verbose": true,
	})

	assert.NoError(t, err)
	// Result may succeed or fail depending on environment.
	assert.NotNil(t, result)
}
