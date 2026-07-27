package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestInScopeByTagsAndLabels covers the pure matching semantics of the
// tags/labels early-skip gate: tags any-match, labels all-match, combined
// filters, an empty selector (always in scope), and templated metadata
// (undecidable). These mirror pkg/scheduler/adapters/terraform.go's
// matchesTerraformTagsAndLabels post-filter exactly (both consume
// pkg/tags.MatchesTags/MatchesLabels), so the early gate here and that later
// authoritative filter can never disagree.
func TestInScopeByTagsAndLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		metadata      map[string]any
		filterTags    []string
		filterLabels  map[string]string
		wantInScope   bool
		wantDecidable bool
	}{
		{
			name:          "empty_selector_always_in_scope",
			metadata:      map[string]any{},
			wantInScope:   true,
			wantDecidable: true,
		},
		{
			name: "tags_any_match_hit",
			metadata: map[string]any{
				"tags": []any{"team-a", "prod"},
			},
			filterTags:    []string{"prod", "staging"},
			wantInScope:   true,
			wantDecidable: true,
		},
		{
			name: "tags_any_match_miss",
			metadata: map[string]any{
				"tags": []any{"team-a", "dev"},
			},
			filterTags:    []string{"prod", "staging"},
			wantInScope:   false,
			wantDecidable: true,
		},
		{
			name: "labels_all_match_hit",
			metadata: map[string]any{
				"labels": map[string]any{"team": "platform", "env": "prod"},
			},
			filterLabels:  map[string]string{"team": "platform"},
			wantInScope:   true,
			wantDecidable: true,
		},
		{
			name: "labels_all_match_miss_partial",
			metadata: map[string]any{
				"labels": map[string]any{"team": "platform"},
			},
			filterLabels:  map[string]string{"team": "platform", "env": "prod"},
			wantInScope:   false,
			wantDecidable: true,
		},
		{
			name: "combined_tags_and_labels_both_must_match",
			metadata: map[string]any{
				"tags":   []any{"prod"},
				"labels": map[string]any{"team": "platform"},
			},
			filterTags:    []string{"prod"},
			filterLabels:  map[string]string{"team": "platform"},
			wantInScope:   true,
			wantDecidable: true,
		},
		{
			name: "combined_tags_match_but_labels_miss",
			metadata: map[string]any{
				"tags":   []any{"prod"},
				"labels": map[string]any{"team": "other"},
			},
			filterTags:    []string{"prod"},
			filterLabels:  map[string]string{"team": "platform"},
			wantInScope:   false,
			wantDecidable: true,
		},
		{
			name: "templated_tags_undecidable",
			metadata: map[string]any{
				"tags": []any{"{{ .vars.stage }}"},
			},
			filterTags:    []string{"prod"},
			wantInScope:   true, // caller must fall through to full evaluation, never drop.
			wantDecidable: false,
		},
		{
			name: "templated_labels_undecidable",
			metadata: map[string]any{
				"labels": map[string]any{"env": "{{ .vars.stage }}"},
			},
			filterLabels:  map[string]string{"env": "prod"},
			wantInScope:   true,
			wantDecidable: false,
		},
		{
			name: "templated_metadata_ignored_when_selector_empty",
			metadata: map[string]any{
				"tags": []any{"{{ .vars.stage }}"},
			},
			wantInScope:   true,
			wantDecidable: true,
		},
		{
			name: "yaml_function_tags_undecidable",
			// Unprocessed Atmos YAML functions are stored as plain strings like
			// "!env DEPLOY_TIER" (see getValueWithTag in pkg/utils); the gate must
			// treat them as unresolved, not as a static non-match.
			metadata: map[string]any{
				"tags": []any{"!env DEPLOY_TIER"},
			},
			filterTags:    []string{"prod"},
			wantInScope:   true,
			wantDecidable: false,
		},
		{
			name: "yaml_function_labels_undecidable",
			metadata: map[string]any{
				"labels": map[string]any{"env": "!store ssm env"},
			},
			filterLabels:  map[string]string{"env": "prod"},
			wantInScope:   true,
			wantDecidable: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inScope, decidable := inScopeByTagsAndLabels(tc.metadata, tc.filterTags, tc.filterLabels)
			assert.Equal(t, tc.wantInScope, inScope, "inScope")
			assert.Equal(t, tc.wantDecidable, decidable, "decidable")
		})
	}
}

// componentSectionWithTags returns a component section carrying
// metadata.tags/metadata.labels plus a default-identity auth section, so that
// resolveComponentAuthManager's own hasDefaultIdentity gate (see
// describe_stacks_component_processor.go) does not itself short-circuit before
// the auth resolver spy is reached — matching componentSectionWithAuth's shape.
func componentSectionWithTags(tags []any, labels map[string]any) map[string]any {
	metadata := map[string]any{}
	if tags != nil {
		metadata["tags"] = tags
	}
	if labels != nil {
		metadata["labels"] = labels
	}
	section := componentSectionWithAuth(authSectionWithDefault())
	section["metadata"] = metadata
	return section
}

// TestProcessComponentEntry_TagsLabelsOutOfScopeSkipsAuth verifies that a
// component excluded by --tags/--labels never triggers per-component auth
// resolution — generalizing TestProcessComponentEntry_OutOfScopeSkipsAuth (the
// -s early-skip) to the tags/labels early-skip gate. Both must run before
// resolveComponentAuthManager so a mismatched component in an unreachable
// account never authenticates. See
// docs/fixes/2026-07-25-scope-before-evaluate-labels-tags-list-dependencies.md.
func TestProcessComponentEntry_TagsLabelsOutOfScopeSkipsAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		tagsFilter   []string
		labelsFilter map[string]string
		componentTag []any
		componentLbl map[string]any
	}{
		{
			name:         "tags_mismatch",
			tagsFilter:   []string{"prod"},
			componentTag: []any{"dev"},
		},
		{
			name:         "labels_mismatch",
			labelsFilter: map[string]string{"team": "platform"},
			componentLbl: map[string]any{"team": "other"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The spy returns an error, so if the resolver were invoked the call would
			// surface as a non-nil error from processComponentEntry. An out-of-scope
			// component must skip it entirely.
			spy := &componentAuthResolverSpy{
				returnMgr: nil,
				returnErr: assert.AnError,
			}

			componentSection := componentSectionWithTags(tc.componentTag, tc.componentLbl)
			allTypeComponents := map[string]any{"test-component": componentSection}
			p := &describeStacksProcessor{
				atmosConfig:           &schema.AtmosConfiguration{},
				tagsFilter:            tc.tagsFilter,
				labelsFilter:          tc.labelsFilter,
				processTemplates:      true,
				processYamlFunctions:  false,
				componentAuthResolver: spy.resolver(),
				finalStacksMap:        make(map[string]any),
			}

			err := p.processComponentEntry(
				"test-stack",
				"",
				"helmfile",
				"test-component",
				componentSection,
				allTypeComponents,
				processComponentTypeOpts{},
			)

			require.NoError(t, err, "tags/labels-excluded component must be skipped without error")
			require.EqualValues(t, 0, spy.calls.Load(), "per-component auth must not run for a tags/labels-excluded component")
			assert.Empty(t, p.finalStacksMap, "no entry should be created for an excluded component")
		})
	}
}

// TestProcessComponentEntry_TagsLabelsInScopeResolvesAuth is the positive
// counterpart: a component matching the tags/labels filter proceeds to normal
// auth resolution (the spy IS invoked).
func TestProcessComponentEntry_TagsLabelsInScopeResolvesAuth(t *testing.T) {
	t.Parallel()

	spy := &componentAuthResolverSpy{returnErr: nil}
	componentSection := componentSectionWithTags([]any{"prod"}, nil)
	allTypeComponents := map[string]any{"test-component": componentSection}
	p := &describeStacksProcessor{
		atmosConfig:           &schema.AtmosConfiguration{},
		tagsFilter:            []string{"prod"},
		processTemplates:      true,
		processYamlFunctions:  false,
		componentAuthResolver: spy.resolver(),
		finalStacksMap:        make(map[string]any),
	}

	err := p.processComponentEntry(
		"test-stack",
		"",
		"helmfile",
		"test-component",
		componentSection,
		allTypeComponents,
		processComponentTypeOpts{},
	)

	require.NoError(t, err)
	require.EqualValues(t, 1, spy.calls.Load(), "per-component auth must run for a tags-matched component")
}

// TestProcessComponentEntry_TagsLabelsTemplatedMetadataFallsThroughToAuth
// verifies the decidable=false safety net: when metadata.tags/metadata.labels
// contain an unresolved template marker, the early gate must NOT skip the
// component — it falls through to full evaluation (auth resolver runs) so a
// possible false-exclusion never silently drops a real dependency. Correctness
// is preserved at the cost of the perf optimization for that one component;
// pkg/scheduler/adapters/terraform.go's post-filter remains authoritative.
func TestProcessComponentEntry_TagsLabelsTemplatedMetadataFallsThroughToAuth(t *testing.T) {
	t.Parallel()

	spy := &componentAuthResolverSpy{returnErr: nil}
	componentSection := componentSectionWithTags([]any{"{{ .vars.stage }}"}, nil)
	allTypeComponents := map[string]any{"test-component": componentSection}
	p := &describeStacksProcessor{
		atmosConfig:           &schema.AtmosConfiguration{},
		tagsFilter:            []string{"prod"},
		processTemplates:      true,
		processYamlFunctions:  false,
		componentAuthResolver: spy.resolver(),
		finalStacksMap:        make(map[string]any),
	}

	err := p.processComponentEntry(
		"test-stack",
		"",
		"helmfile",
		"test-component",
		componentSection,
		allTypeComponents,
		processComponentTypeOpts{},
	)

	require.NoError(t, err)
	require.EqualValues(t, 1, spy.calls.Load(), "templated selector metadata must fall through to full evaluation, not be skipped")
}

// TestProcessComponentEntry_TagsLabelsEagerEvaluationOverride verifies the
// describe.settings.eager_evaluation rollback: with it set, a
// tag-mismatched component is still fully evaluated (auth resolver runs)
// instead of being skipped by the early gate — an instant escape hatch if a
// repo's templated metadata isn't safely detectable by tags.SelectorUnresolved.
func TestProcessComponentEntry_TagsLabelsEagerEvaluationOverride(t *testing.T) {
	t.Parallel()

	eager := true
	spy := &componentAuthResolverSpy{returnErr: nil}
	componentSection := componentSectionWithTags([]any{"dev"}, nil) // mismatches "prod" below.
	allTypeComponents := map[string]any{"test-component": componentSection}
	p := &describeStacksProcessor{
		atmosConfig: &schema.AtmosConfiguration{
			Describe: schema.Describe{
				Settings: schema.DescribeSettings{EagerEvaluation: &eager},
			},
		},
		tagsFilter:            []string{"prod"},
		processTemplates:      true,
		processYamlFunctions:  false,
		componentAuthResolver: spy.resolver(),
		finalStacksMap:        make(map[string]any),
	}

	err := p.processComponentEntry(
		"test-stack",
		"",
		"helmfile",
		"test-component",
		componentSection,
		allTypeComponents,
		processComponentTypeOpts{},
	)

	require.NoError(t, err)
	require.EqualValues(t, 1, spy.calls.Load(), "eager_evaluation=true must force full evaluation even for a mismatched component")
}
