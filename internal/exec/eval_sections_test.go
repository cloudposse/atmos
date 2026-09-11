package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsSectionRequired(t *testing.T) {
	tests := []struct {
		name        string
		sections    []string
		sectionName string
		want        bool
	}{
		{name: "nil filter requires everything (vars)", sections: nil, sectionName: "vars", want: true},
		{name: "nil filter requires everything (settings)", sections: nil, sectionName: "settings", want: true},
		{name: "empty non-nil filter requires nothing", sections: []string{}, sectionName: "vars", want: false},
		{name: "non-nil filter matches member", sections: []string{"vars"}, sectionName: "vars", want: true},
		{name: "non-nil filter rejects non-member", sections: []string{"vars"}, sectionName: "settings", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSectionRequired(tt.sections, tt.sectionName))
		})
	}
}

func TestSplitSectionsByRequirement(t *testing.T) {
	componentSection := map[string]any{
		"vars":     map[string]any{"region": "us-east-1"},
		"settings": map[string]any{"foo": "bar"},
		"metadata": map[string]any{"type": "real"},
	}

	t.Run("nil filter returns the input unchanged and a nil excluded map", func(t *testing.T) {
		filtered, excluded := splitSectionsByRequirement(componentSection, nil)
		assert.Equal(t, componentSection, filtered)
		assert.Nil(t, excluded)
	})

	t.Run("empty filter excludes everything", func(t *testing.T) {
		filtered, excluded := splitSectionsByRequirement(componentSection, []string{})
		assert.Empty(t, filtered)
		assert.Equal(t, componentSection, excluded)
	})

	t.Run("filter splits required from excluded", func(t *testing.T) {
		filtered, excluded := splitSectionsByRequirement(componentSection, []string{"vars"})
		assert.Equal(t, map[string]any{"vars": componentSection["vars"]}, filtered)
		assert.Equal(t, map[string]any{
			"settings": componentSection["settings"],
			"metadata": componentSection["metadata"],
		}, excluded)
	})
}

// TestProcessComponentSectionTemplates_EvalSections_SkipsExcludedSection proves the fast path
// added to processComponentSectionTemplates: when evalSections excludes every section, template
// rendering (ProcessTmplWithDatasources) is skipped entirely and every section is returned
// byte-for-byte untouched -- including a literal `{{ }}` expression, which would otherwise have
// been rendered (and, if it were an atmos.Component call, executed).
func TestProcessComponentSectionTemplates_EvalSections_SkipsExcludedSection(t *testing.T) {
	ac := templatingEnabledConfig()
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}
	componentSection := map[string]any{
		"vars": map[string]any{
			"region": "{{ .settings.owner }}", // would render to "acme" if evaluated.
		},
		"settings": map[string]any{
			"owner": "acme",
		},
	}

	result, err := processComponentSectionTemplates(ac, info, componentSection, map[string]any{}, []string{})
	require.NoError(t, err)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "{{ .settings.owner }}", vars["region"], "vars was excluded from evalSections, so its template must never be rendered")
}

// TestProcessComponentSectionTemplates_EvalSections_RendersRequiredSection proves the
// complementary case: a required section IS rendered normally, and can read a skipped sibling
// section's raw (unrendered) value through the template context, which is always built from the
// full, unfiltered componentSection -- see processComponentSectionTemplates' doc comment. This is
// the accepted cross-section limitation: since `settings` is excluded and therefore never
// rendered, `owner` in the context is whatever raw value it already had (here a plain string, but
// see the YAML-tag variant below for the more interesting case).
func TestProcessComponentSectionTemplates_EvalSections_RendersRequiredSection(t *testing.T) {
	ac := templatingEnabledConfig()
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}
	componentSection := map[string]any{
		"vars": map[string]any{
			"region": "{{ .settings.owner }}",
		},
		"settings": map[string]any{
			"owner": "acme",
		},
	}

	result, err := processComponentSectionTemplates(ac, info, componentSection, map[string]any{}, []string{"vars"})
	require.NoError(t, err)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", vars["region"], "vars is required, so its template renders normally against the full (unfiltered) context")

	settings, ok := result["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "acme", settings["owner"], "settings was excluded from evalSections, so it must be restored completely untouched")
}

// TestProcessComponentSectionTemplates_EvalSections_CrossSectionLimitation is the dedicated test
// for the accepted limitation documented on processComponentSectionTemplates: a required
// section's template reads a field from a skipped section, and that field's raw value is itself
// an un-rendered `!terraform.state` YAML-function string (i.e. the skipped section needed
// processing too, but was never touched because it wasn't required). The required section's
// output contains that literal unresolved substring instead of the real backend value -- the same
// "deferred but never resolved = literal string" class of behavior already documented at
// internal/exec/stack_processor_merge.go:114-122, reached here via a new path (evalSections
// gating) rather than a new failure mode. This must not panic or corrupt any other section.
func TestProcessComponentSectionTemplates_EvalSections_CrossSectionLimitation(t *testing.T) {
	ac := templatingEnabledConfig()
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}
	const unresolvedTag = "!terraform.state vpc dev bucket_name"
	componentSection := map[string]any{
		"vars": map[string]any{
			"combined": "{{ .settings.foo }}-suffix",
		},
		"settings": map[string]any{
			// settings is excluded below, so Stage 2 (YAML-tag resolution) never runs over
			// this value either -- it stays exactly this literal string throughout.
			"foo": unresolvedTag,
		},
		"metadata": map[string]any{
			"owner": "unaffected",
		},
	}

	result, err := processComponentSectionTemplates(ac, info, componentSection, map[string]any{}, []string{"vars"})
	require.NoError(t, err)

	vars, ok := result["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, unresolvedTag+"-suffix", vars["combined"],
		"required vars template reads .settings.foo -- since settings was skipped, the raw unresolved YAML-tag string is substituted literally")

	settings, ok := result["settings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, unresolvedTag, settings["foo"], "the skipped settings section itself must be restored completely untouched")

	metadata, ok := result["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "unaffected", metadata["owner"], "an unrelated skipped section must not be corrupted by the cross-section read")
}

// TestExecuteDescribeStacksWithEvalSections_SkipsUnrequiredSection is the exec-layer proof behind
// the `list stacks`/`list components`/`list instances` fix for
// https://github.com/cloudposse/atmos/issues/3068 and the spurious "(computed)" warning: when
// evalSections excludes "vars" (mirroring a default `list stacks` invocation, whose only column
// is `{{ .stack }}`), the `!terraform.state` call embedded in vars.bucket is never invoked at all
// -- zero degradation warnings AND zero backend calls (proven via the mock's strict
// unexpected-call failure, not just a Times(0) on one arg combination) -- while a filter that DOES
// require "vars" (mirroring `--columns` referencing `.vars`) degrades exactly like the unfiltered
// (nil evalSections) path.
func TestExecuteDescribeStacksWithEvalSections_SkipsUnrequiredSection(t *testing.T) {
	atmosConfig := buildDescribeStacksDegradationFixture(t)

	t.Run("evalSections excludes vars: zero warnings, GetState never called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStateGetter := NewMockTerraformStateGetter(ctrl)
		originalGetter := stateGetter
		stateGetter = mockStateGetter
		defer func() { stateGetter = originalGetter }()
		// No EXPECT() registered on GetState: gomock fails the test on any unexpected call,
		// which is exactly the assertion this test needs -- the expensive backend lookup must
		// never run at all when no column requires `vars`.

		var warnings []DegradationWarning
		result, err := ExecuteDescribeStacksWithEvalSections(
			&atmosConfig, "", nil, nil, nil, false,
			false, // processTemplates
			true,  // processYamlFunctions
			false, // includeEmptyStacks
			nil, nil, false, nil, nil,
			DescribeStacksErrorOptions{
				OnError:   OnErrorWarn,
				OnWarning: func(w DegradationWarning) { warnings = append(warnings, w) },
			},
			[]string{}, // evalSections: no section required, mirrors default `list stacks` columns.
		)

		require.NoError(t, err)
		assert.Empty(t, warnings, "no column needs `vars`, so the !terraform.state call must never even run")
		assert.NotEmpty(t, result)
	})

	t.Run("evalSections includes vars: degrades exactly like the unfiltered path", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStateGetter := NewMockTerraformStateGetter(ctrl)
		originalGetter := stateGetter
		stateGetter = mockStateGetter
		defer func() { stateGetter = originalGetter }()

		recoverableErr := fmt.Errorf("%w for component `vpc` in stack `dev`", errUtils.ErrTerraformStateNotProvisioned)
		mockStateGetter.EXPECT().
			GetState(gomock.Any(), gomock.Any(), "dev", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
			Return(nil, recoverableErr).
			Times(1)

		var warnings []DegradationWarning
		_, err := ExecuteDescribeStacksWithEvalSections(
			&atmosConfig, "", nil, nil, nil, false,
			false, true, false, nil, nil, false, nil, nil,
			DescribeStacksErrorOptions{
				OnError:   OnErrorWarn,
				OnWarning: func(w DegradationWarning) { warnings = append(warnings, w) },
			},
			[]string{"vars"},
		)

		require.NoError(t, err)
		require.Len(t, warnings, 1)
	})

	// Regression: nil evalSections (every existing caller -- describe stacks, terraform,
	// helmfile, hooks, etc.) must behave identically to the pre-existing, unfiltered
	// ExecuteDescribeStacksWithOptions path (see
	// TestExecuteDescribeStacks_OnErrorWarn_DegradesRecoverableError) -- full eager evaluation,
	// one degradation warning, no gating whatsoever.
	t.Run("nil evalSections: identical to full eager evaluation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStateGetter := NewMockTerraformStateGetter(ctrl)
		originalGetter := stateGetter
		stateGetter = mockStateGetter
		defer func() { stateGetter = originalGetter }()

		recoverableErr := fmt.Errorf("%w for component `vpc` in stack `dev`", errUtils.ErrTerraformStateNotProvisioned)
		mockStateGetter.EXPECT().
			GetState(gomock.Any(), gomock.Any(), "dev", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
			Return(nil, recoverableErr).
			Times(1)

		var warnings []DegradationWarning
		_, err := ExecuteDescribeStacksWithEvalSections(
			&atmosConfig, "", nil, nil, nil, false,
			false, true, false, nil, nil, false, nil, nil,
			DescribeStacksErrorOptions{
				OnError:   OnErrorWarn,
				OnWarning: func(w DegradationWarning) { warnings = append(warnings, w) },
			},
			nil,
		)

		require.NoError(t, err)
		require.Len(t, warnings, 1, "nil evalSections must reproduce the exact eager-evaluation behavior of every other Execute* variant")
	})
}
