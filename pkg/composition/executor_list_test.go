package composition

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestListRows_AggregatesAvailableStacks(t *testing.T) {
	comps := map[string]schema.Composition{
		"app":      {Description: "Application", Services: []string{"api", "database"}},
		"payments": {Description: "Payments", Services: []string{"worker"}},
		"empty":    {Description: "Declared but not fulfilled", Services: []string{"cache"}},
	}
	stacks := map[string]any{
		"prod":  stackWithComponents(map[string]string{"api": "app"}),
		"local": stackWithComponents(map[string]string{"api": "app", "worker": "payments"}),
	}
	withValidateStubs(t, comps, stacks, nil, nil)

	rows, err := ListRows(context.Background(), &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Equal(t, []map[string]any{
		{"composition": "app", "services": "api, database", "stacks": "local, prod", "description": "Application"},
		{"composition": "empty", "services": "cache", "stacks": "", "description": "Declared but not fulfilled"},
		{"composition": "payments", "services": "worker", "stacks": "local", "description": "Payments"},
	}, rows)
}

func TestListRows_ReportsStackFulfillment(t *testing.T) {
	comps := map[string]schema.Composition{
		"app": {Description: "Application", Services: []string{"api", "database"}},
	}
	stacks := map[string]any{
		"local": stackWithComponents(map[string]string{"api": "app", "sidecar": "app"}),
		"prod":  stackWithComponents(map[string]string{"api": "app", "database": "app"}),
	}
	withValidateStubs(t, comps, stacks, nil, nil)

	rows, err := ListRows(context.Background(), &schema.ConfigAndStacksInfo{Stack: "local"})
	require.NoError(t, err)
	assert.Equal(t, []map[string]any{
		{
			"composition": "app", "services": "api, database", "fulfilled": "api",
			"not_provided": "database", "unknown": "container.sidecar", "description": "Application",
		},
	}, rows)
}

func TestListRows_EmptyCompositions(t *testing.T) {
	withValidateStubs(t, map[string]schema.Composition{}, map[string]any{}, nil, nil)

	rows, err := ListRows(context.Background(), &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestListRows_ListsUnfulfilledDeclarationsWithoutStacks(t *testing.T) {
	comps := map[string]schema.Composition{
		"app": {Description: "Application", Services: []string{"api"}},
	}
	withValidateStubs(t, comps, nil, nil, errUtils.ErrNoStackManifestsFound)

	rows, err := ListRows(context.Background(), &schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	assert.Equal(t, []map[string]any{{
		"composition": "app", "services": "api", "stacks": "", "description": "Application",
	}}, rows)
}
