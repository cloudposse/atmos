package exec

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Dynamic dependencies can include template settings needed by a later rendering
// pass. The unfiltered raw context alone does not evaluate those settings.
func TestDynamicEvaluationPreservesCrossSectionTemplateSettings(t *testing.T) {
	const envKey = "ATMOS_TEST_DYNAMIC_EVALUATION_TEAM"
	t.Setenv(envKey, "original")
	ac := templatingEnabledConfig()
	ac.Templates.Settings.Evaluations = 2
	ac.Templates.Settings.Sprig.Enabled = true
	authdeferred.ConfigureAuth(ac, "")
	setDeferredAuthFactory(ac, authdeferred.NewMockAuthFactory(gomock.NewController(t)))
	settings := map[string]any{
		"owner": `{{ env "ATMOS_TEST_DYNAMIC_EVALUATION_TEAM" }}`,
		"templates": map[string]any{"settings": map[string]any{
			"env": map[string]any{envKey: "{{ .vars.literal }}"},
		}},
	}
	input := map[string]any{
		"vars": map[string]any{
			"literal": "platform",
			"name":    `{{ index . "settings" "owner" }}`,
		},
		"settings": settings,
	}
	paths := [][]string{{"vars", "name"}}
	info := &schema.ConfigAndStacksInfo{
		ComponentSection: input,
		EvaluationPaths:  deferred.ExpandEvaluationPaths(input, paths),
	}
	require.Nil(t, info.EvaluationPaths, "dynamic root access needs conservative evaluation")
	processor := &describeStacksProcessor{evalSections: []string{"vars"}, evalPaths: paths}
	result, err := processComponentSectionTemplates(ac, info, input, settings, processor.evaluationSections(info))
	require.NoError(t, err)
	vars := result["vars"].(map[string]any)
	require.Equal(t, "platform", vars["name"], "the selected value needs settings rendered before its second pass")
}
