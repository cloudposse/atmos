package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

// closureTestStacksMap builds a described-stacks map with a cross-tag,
// cross-stack dependency chain: dev/app (tags: app) depends on dev/db
// (tags: data), which depends on core/vpc (tags: network).
func closureTestStacksMap() map[string]any {
	component := func(tags []any, dependsOn map[string]any) map[string]any {
		section := map[string]any{
			"metadata": map[string]any{"tags": tags},
			"vars":     map[string]any{},
		}
		if dependsOn != nil {
			section["settings"] = map[string]any{"depends_on": dependsOn}
		}
		return section
	}
	return map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"app": component([]any{"app"}, map[string]any{"1": map[string]any{"component": "db"}}),
					"db":  component([]any{"data"}, map[string]any{"1": map[string]any{"component": "vpc", "stack": "core"}}),
				},
			},
		},
		"core": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": component([]any{"network"}, nil),
				},
			},
		},
		"unrelated": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"poison": component([]any{"other"}, nil),
				},
			},
		},
	}
}

// TestBuildInstanceClosureFilter verifies the closure membership filter keeps
// prerequisites that do not match the seeding selectors, and drops everything
// outside the closure.
func TestBuildInstanceClosureFilter(t *testing.T) {
	t.Parallel()

	rows := []map[string]any{
		{"component": "app", "stack": "dev"},
		{"component": "db", "stack": "dev"},
		{"component": "vpc", "stack": "core"},
		{"component": "poison", "stack": "unrelated"},
	}

	tests := []struct {
		name           string
		opts           *InstancesCommandOptions
		wantComponents []string
	}{
		{
			name: "tags seed with unlimited dependencies keeps non-matching prereqs",
			opts: &InstancesCommandOptions{
				Tags:                []string{"app"},
				IncludeDependencies: -1,
			},
			wantComponents: []string{"app", "db", "vpc"},
		},
		{
			name: "depth 1 stops one level deep",
			opts: &InstancesCommandOptions{
				Tags:                []string{"app"},
				IncludeDependencies: 1,
			},
			wantComponents: []string{"app", "db"},
		},
		{
			name: "dependents direction from the network seed",
			opts: &InstancesCommandOptions{
				Tags:              []string{"network"},
				IncludeDependents: -1,
			},
			wantComponents: []string{"app", "db", "vpc"},
		},
		{
			name: "stack glob seed",
			opts: &InstancesCommandOptions{
				Stack:               "de*",
				IncludeDependencies: -1,
			},
			wantComponents: []string{"app", "db", "vpc"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			closureFilter, err := buildInstanceClosureFilter(
				&schema.AtmosConfiguration{}, tc.opts, nil, closureTestStacksMap(),
			)
			require.NoError(t, err)

			filtered, err := closureFilter.Apply(rows)
			require.NoError(t, err)
			filteredRows, ok := filtered.([]map[string]any)
			require.True(t, ok)

			var components []string
			for _, row := range filteredRows {
				components = append(components, row["component"].(string))
			}
			assert.ElementsMatch(t, tc.wantComponents, components)
		})
	}
}

// TestExecuteDescribeStacksForInstances_ScopedDispatch verifies that a
// tags/labels filter routes to the scoped describe variant (the early-skip
// gate) when the processor implements it, and that no filter keeps the
// regular path.
func TestExecuteDescribeStacksForInstances_ScopedDispatch(t *testing.T) {
	t.Parallel()

	t.Run("tags filter routes to scoped variant", func(t *testing.T) {
		t.Parallel()
		fake := &scopedFakeProcessor{}
		_, err := executeDescribeStacksForInstances(
			&schema.AtmosConfiguration{}, fake, nil, true, true, nil, false,
			[]string{"app"}, nil,
		)
		require.NoError(t, err)
		assert.True(t, fake.scopedCalled, "a tags filter must dispatch to ExecuteDescribeStacksScoped")
		assert.False(t, fake.regularCalled)
		assert.Equal(t, []string{"app"}, fake.capturedTags)
	})

	t.Run("labels filter routes to scoped variant", func(t *testing.T) {
		t.Parallel()
		fake := &scopedFakeProcessor{}
		labels := map[string]string{"env": "dev"}
		_, err := executeDescribeStacksForInstances(
			&schema.AtmosConfiguration{}, fake, nil, true, true, nil, false,
			nil, labels,
		)
		require.NoError(t, err)
		assert.True(t, fake.scopedCalled, "a labels filter must dispatch to ExecuteDescribeStacksScoped")
		assert.False(t, fake.regularCalled)
		assert.Equal(t, labels, fake.capturedLabels)
	})

	t.Run("no filter keeps the regular path", func(t *testing.T) {
		t.Parallel()
		fake := &scopedFakeProcessor{}
		_, err := executeDescribeStacksForInstances(
			&schema.AtmosConfiguration{}, fake, nil, true, true, nil, false,
			nil, nil,
		)
		require.NoError(t, err)
		assert.False(t, fake.scopedCalled)
		assert.True(t, fake.regularCalled, "without a filter the regular describe path must be used")
	})
}

// scopedFakeProcessor implements StacksProcessor plus the optional
// scopedStacksProcessor interface, recording which path was taken.
type scopedFakeProcessor struct {
	scopedCalled   bool
	regularCalled  bool
	capturedTags   []string
	capturedLabels map[string]string
}

func (f *scopedFakeProcessor) ExecuteDescribeStacks(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
) (map[string]any, error) {
	f.regularCalled = true
	return map[string]any{}, nil
}

func (f *scopedFakeProcessor) ExecuteDescribeStacksScoped(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
	authDisabled bool,
	tagsFilter []string,
	labelsFilter map[string]string,
) (map[string]any, error) {
	f.scopedCalled = true
	f.capturedTags = tagsFilter
	f.capturedLabels = labelsFilter
	return map[string]any{}, nil
}
