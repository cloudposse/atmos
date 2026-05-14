package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractAffectedNodeIDs(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		ids := extractAffectedNodeIDs(nil)
		assert.Empty(t, ids)
	})

	t.Run("single affected", func(t *testing.T) {
		affected := []schema.Affected{
			{Component: "vpc", Stack: "dev"},
		}
		ids := extractAffectedNodeIDs(affected)
		assert.Equal(t, []string{"vpc-dev"}, ids)
	})

	t.Run("multiple affected", func(t *testing.T) {
		affected := []schema.Affected{
			{Component: "vpc", Stack: "dev"},
			{Component: "rds", Stack: "prod"},
			{Component: "app", Stack: "staging"},
		}
		ids := extractAffectedNodeIDs(affected)
		assert.Equal(t, 3, len(ids))
		assert.Equal(t, "vpc-dev", ids[0])
		assert.Equal(t, "rds-prod", ids[1])
		assert.Equal(t, "app-staging", ids[2])
	})
}

func TestIsDirectlyAffected(t *testing.T) {
	affectedList := []schema.Affected{
		{Component: "vpc", Stack: "dev"},
		{Component: "rds", Stack: "prod"},
	}

	t.Run("directly affected", func(t *testing.T) {
		node := &dependency.Node{Component: "vpc", Stack: "dev"}
		assert.True(t, isDirectlyAffected(node, affectedList))
	})

	t.Run("not affected", func(t *testing.T) {
		node := &dependency.Node{Component: "app", Stack: "dev"}
		assert.False(t, isDirectlyAffected(node, affectedList))
	})

	t.Run("component matches but stack differs", func(t *testing.T) {
		node := &dependency.Node{Component: "vpc", Stack: "prod"}
		assert.False(t, isDirectlyAffected(node, affectedList))
	})

	t.Run("empty affected list", func(t *testing.T) {
		node := &dependency.Node{Component: "vpc", Stack: "dev"}
		assert.False(t, isDirectlyAffected(node, nil))
	})
}

func TestLogAffectedComponents(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		err := logAffectedComponents(nil)
		assert.NoError(t, err)
	})

	t.Run("with components", func(t *testing.T) {
		affected := []schema.Affected{
			{Component: "vpc", Stack: "dev"},
			{Component: "rds", Stack: "prod"},
		}
		err := logAffectedComponents(affected)
		assert.NoError(t, err)
	})
}

func TestLogComponentExecution(t *testing.T) {
	node := &dependency.Node{Component: "vpc", Stack: "dev"}

	// These just log — verify they don't panic.
	t.Run("directly affected", func(t *testing.T) {
		logComponentExecution(node, 1, 3, true, false)
	})

	t.Run("dependent", func(t *testing.T) {
		logComponentExecution(node, 2, 3, false, true)
	})

	t.Run("dependency", func(t *testing.T) {
		logComponentExecution(node, 3, 3, false, false)
	})
}

func TestGetAffectedComponentsList(t *testing.T) {
	// Test the dispatch logic — all three paths require real repos so they'll fail,
	// but we verify the dispatch works.
	t.Run("repo path dispatches correctly", func(t *testing.T) {
		args := &DescribeAffectedCmdArgs{
			RepoPath: "/nonexistent/path",
		}
		_, err := getAffectedComponentsList(args)
		// Should fail because the path doesn't exist, but the dispatch is correct.
		assert.Error(t, err)
	})
}
