package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestShouldSkipNode(t *testing.T) {
	t.Run("no metadata", func(t *testing.T) {
		node := &dependency.Node{
			ID: "vpc-dev", Component: "vpc", Stack: "dev",
			Metadata: map[string]any{"vars": map[string]any{"key": "val"}},
		}
		assert.False(t, shouldSkipNode(node))
	})

	t.Run("abstract component", func(t *testing.T) {
		node := &dependency.Node{
			ID: "base-dev", Component: "base", Stack: "dev",
			Metadata: map[string]any{
				cfg.MetadataSectionName: map[string]any{"type": "abstract"},
			},
		}
		assert.True(t, shouldSkipNode(node))
	})

	t.Run("disabled component", func(t *testing.T) {
		node := &dependency.Node{
			ID: "vpc-dev", Component: "vpc", Stack: "dev",
			Metadata: map[string]any{
				cfg.MetadataSectionName: map[string]any{"enabled": false},
			},
		}
		assert.True(t, shouldSkipNode(node))
	})

	t.Run("enabled component", func(t *testing.T) {
		node := &dependency.Node{
			ID: "vpc-dev", Component: "vpc", Stack: "dev",
			Metadata: map[string]any{
				cfg.MetadataSectionName: map[string]any{"enabled": true},
			},
		}
		assert.False(t, shouldSkipNode(node))
	})

	t.Run("no metadata section key", func(t *testing.T) {
		node := &dependency.Node{
			ID: "vpc-dev", Component: "vpc", Stack: "dev",
			Metadata: map[string]any{},
		}
		assert.False(t, shouldSkipNode(node))
	})
}

func TestIsAbstractMetadata(t *testing.T) {
	assert.True(t, isAbstractMetadata(map[string]any{"type": "abstract"}))
	assert.False(t, isAbstractMetadata(map[string]any{"type": "real"}))
	assert.False(t, isAbstractMetadata(map[string]any{}))
	assert.False(t, isAbstractMetadata(map[string]any{"type": 42}))
}

func TestIsNodeComponentEnabled(t *testing.T) {
	assert.True(t, isNodeComponentEnabled(map[string]any{}, "vpc"))
	assert.True(t, isNodeComponentEnabled(map[string]any{"enabled": true}, "vpc"))
	assert.False(t, isNodeComponentEnabled(map[string]any{"enabled": false}, "vpc"))
	assert.True(t, isNodeComponentEnabled(map[string]any{"enabled": "yes"}, "vpc")) // Non-bool stays enabled.
}

func TestShouldSkipByQuery(t *testing.T) {
	t.Run("empty query never skips", func(t *testing.T) {
		node := &dependency.Node{ID: "vpc-dev", Metadata: map[string]any{"key": "val"}}
		info := &schema.ConfigAndStacksInfo{Query: ""}
		assert.False(t, shouldSkipByQuery(node, info))
	})

	t.Run("nil metadata never skips", func(t *testing.T) {
		node := &dependency.Node{ID: "vpc-dev", Metadata: nil}
		info := &schema.ConfigAndStacksInfo{Query: ".enabled"}
		assert.False(t, shouldSkipByQuery(node, info))
	})
}

func TestUpdateInfoFromNode(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{}
	node := &dependency.Node{ID: "vpc-dev", Component: "vpc", Stack: "dev"}

	updateInfoFromNode(info, node)

	assert.Equal(t, "vpc", info.Component)
	assert.Equal(t, "vpc", info.ComponentFromArg)
	assert.Equal(t, "dev", info.Stack)
	assert.Equal(t, "dev", info.StackFromArg)
}

func TestFormatNodeCommand(t *testing.T) {
	node := &dependency.Node{ID: "vpc-dev", Component: "vpc", Stack: "dev"}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan"}

	result := formatNodeCommand(node, info)
	assert.Equal(t, "atmos terraform plan vpc -s dev", result)
}

func TestExecuteNodeCommand_DryRun(t *testing.T) {
	node := &dependency.Node{ID: "vpc-dev", Component: "vpc", Stack: "dev"}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan", DryRun: true}

	// Dry run should return nil without executing anything.
	err := executeNodeCommand(node, info)
	assert.NoError(t, err)
}

func TestExecuteTerraformForNode_SkipsAbstract(t *testing.T) {
	node := &dependency.Node{
		ID: "base-dev", Component: "base", Stack: "dev",
		Metadata: map[string]any{
			cfg.MetadataSectionName: map[string]any{"type": "abstract"},
		},
	}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan"}

	// Should skip without error.
	err := executeTerraformForNode(node, info)
	assert.NoError(t, err)
}

func TestExecuteTerraformForNode_SkipsDisabled(t *testing.T) {
	node := &dependency.Node{
		ID: "vpc-dev", Component: "vpc", Stack: "dev",
		Metadata: map[string]any{
			cfg.MetadataSectionName: map[string]any{"enabled": false},
		},
	}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan"}

	err := executeTerraformForNode(node, info)
	assert.NoError(t, err)
}

func TestExecuteTerraformForNode_DryRun(t *testing.T) {
	node := &dependency.Node{
		ID: "vpc-dev", Component: "vpc", Stack: "dev",
		Metadata: map[string]any{
			"vars": map[string]any{"key": "val"},
		},
	}
	info := &schema.ConfigAndStacksInfo{SubCommand: "plan", DryRun: true}

	// Dry run should complete without error.
	err := executeTerraformForNode(node, info)
	assert.NoError(t, err)
	// Verify info was updated.
	assert.Equal(t, "vpc", info.Component)
	assert.Equal(t, "dev", info.Stack)
}
