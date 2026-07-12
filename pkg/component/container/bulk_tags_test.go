package container

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// taggedStacks builds a describe-stacks map with three container components
// (dev/api, dev/worker, prod/api), each with distinct metadata.tags/labels.
func taggedStacks() map[string]any {
	component := func(image string, tagsList []any, labels map[string]any) map[string]any {
		return map[string]any{
			"image": image,
			"metadata": map[string]any{
				"tags":   tagsList,
				"labels": labels,
			},
		}
	}

	return map[string]any{
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.ContainerComponentType: map[string]any{
					"api": component(
						"api:dev",
						[]any{"production", "compute"},
						map[string]any{"cost-center": "platform"},
					),
					"worker": component(
						"worker:dev",
						[]any{"development"},
						map[string]any{"cost-center": "data"},
					),
				},
			},
		},
		"prod": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.ContainerComponentType: map[string]any{
					"api": component(
						"api:prod",
						[]any{"production", "networking"},
						map[string]any{"cost-center": "platform", "compliance": "sox"},
					),
				},
			},
		},
	}
}

func TestShouldRunBulk_TagsAndLabels(t *testing.T) {
	assert.True(t, shouldRunBulk("up", &schema.ConfigAndStacksInfo{Tags: []string{"production"}, ComponentFromArg: ""}))
	assert.True(t, shouldRunBulk("up", &schema.ConfigAndStacksInfo{Labels: map[string]string{"cost-center": "platform"}}))
	assert.False(t, shouldRunBulk("logs", &schema.ConfigAndStacksInfo{Tags: []string{"production"}}))
}

func TestExecuteBulk_ComponentWithTagsRejected(t *testing.T) {
	err := ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{Tags: []string{"production"}, ComponentFromArg: "api"}, "up")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrContainerComponentWithAll)
}

func TestExecuteBulk_TagsFilterAcrossStacks(t *testing.T) {
	withListStubs(t, taggedStacks(), nil, nil, nil)

	var calls []string
	withBulkExecutor(t, "up", func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{Tags: []string{"production"}}, "up"))
	assert.Equal(t, []string{"dev/api", "prod/api"}, calls)
}

func TestExecuteBulk_LabelsRequireAllPairs(t *testing.T) {
	withListStubs(t, taggedStacks(), nil, nil, nil)

	var calls []string
	withBulkExecutor(t, "up", func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{
		Labels: map[string]string{"cost-center": "platform", "compliance": "sox"},
	}, "up"))
	assert.Equal(t, []string{"prod/api"}, calls)
}

func TestExecuteBulk_AllComposesWithTags(t *testing.T) {
	withListStubs(t, taggedStacks(), nil, nil, nil)

	var calls []string
	withBulkExecutor(t, "up", func(_ context.Context, info *schema.ConfigAndStacksInfo) error {
		calls = append(calls, info.Stack+"/"+info.ComponentFromArg)
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{
		All:  true,
		Tags: []string{"development"},
	}, "up"))
	assert.Equal(t, []string{"dev/worker"}, calls)
}

func TestExecuteBulk_TagsNoMatchIsNoop(t *testing.T) {
	withListStubs(t, taggedStacks(), nil, nil, nil)

	withBulkExecutor(t, "up", func(_ context.Context, _ *schema.ConfigAndStacksInfo) error {
		t.Fatal("executor must not be called when no components match")
		return nil
	})

	require.NoError(t, ExecuteBulk(context.Background(), &schema.ConfigAndStacksInfo{Tags: []string{"nonexistent"}}, "up"))
}

func TestCollectContainerInstances_ExtractsTagsAndLabels(t *testing.T) {
	rows := collectContainerInstances(taggedStacks())
	require.Len(t, rows, 3)

	byKey := make(map[string]instanceRow, len(rows))
	for _, r := range rows {
		byKey[r.stack+"/"+r.component] = r
	}

	assert.Equal(t, []string{"production", "compute"}, byKey["dev/api"].tags)
	assert.Equal(t, map[string]string{"cost-center": "platform"}, byKey["dev/api"].labels)
	assert.Equal(t, []string{"development"}, byKey["dev/worker"].tags)
	assert.Equal(t, map[string]string{"cost-center": "data"}, byKey["dev/worker"].labels)
	assert.Equal(t, map[string]string{"cost-center": "platform", "compliance": "sox"}, byKey["prod/api"].labels)
}

func TestFilterByTagsAndLabels(t *testing.T) {
	rows := []instanceRow{
		{stack: "dev", component: "api", tags: []string{"production"}, labels: map[string]string{"cost-center": "platform"}},
		{stack: "dev", component: "worker", tags: []string{"development"}, labels: map[string]string{"cost-center": "data"}},
	}

	t.Run("no filter returns rows unchanged", func(t *testing.T) {
		assert.Equal(t, rows, filterByTagsAndLabels(rows, &schema.ConfigAndStacksInfo{}))
	})

	t.Run("tags filter", func(t *testing.T) {
		filtered := filterByTagsAndLabels(rows, &schema.ConfigAndStacksInfo{Tags: []string{"production"}})
		require.Len(t, filtered, 1)
		assert.Equal(t, "api", filtered[0].component)
	})

	t.Run("labels filter", func(t *testing.T) {
		filtered := filterByTagsAndLabels(rows, &schema.ConfigAndStacksInfo{Labels: map[string]string{"cost-center": "data"}})
		require.Len(t, filtered, 1)
		assert.Equal(t, "worker", filtered[0].component)
	})
}
