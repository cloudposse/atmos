package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDeferredAuthUnusedSibling(t *testing.T) {
	ctrl := gomock.NewController(t)
	ac := templatingEnabledConfig()
	deferred.ConfigureAuth(ac, "")
	setDeferredAuthFactory(ac, deferred.NewMockAuthFactory(ctrl))
	input := map[string]any{"vars": map[string]any{
		"literal": "preserved", "account": "!aws.account_id",
		"template": `{{ atmos.Resolve "!aws.account_id" }}`,
	}}
	info := &schema.ConfigAndStacksInfo{Stack: "dev", ComponentSection: input, EvaluationPaths: [][]string{{"vars", "literal"}}}
	errOpts, collector := ErrorOptionsFromMode("warn")
	rendered, err := processComponentSectionTemplates(ac, info, input, nil, []string{"vars"}, errOpts.OnWarning)
	require.NoError(t, err)
	got, err := processComponentSectionYAMLFunctions(ac, info, rendered, nil, errOpts.OnWarning, true, []string{"vars"})
	require.NoError(t, err)
	assert.Equal(t, input, got)
	assert.Zero(t, collector.Count())
}

func TestExpandEvaluationPaths(t *testing.T) {
	input := map[string]any{
		"vars":     map[string]any{"name": "{{ .settings.owner }}", "other": "!aws.account_id"},
		"settings": map[string]any{"owner": "{{ .env.TEAM }}"},
		"env":      map[string]any{"TEAM": "platform"},
	}
	paths := deferred.ExpandEvaluationPaths(input, [][]string{{"vars", "name"}})
	assert.ElementsMatch(t, [][]string{{"vars", "name"}, {"settings", "owner"}, {"env", "TEAM"}}, paths)
	selected, excluded := deferred.SplitEvaluationFields(input, paths)
	assert.NotContains(t, selected["vars"], "other")
	deferred.RestoreEvaluationFields(selected, excluded)
	assert.Equal(t, input, selected)
	assert.Nil(t, deferred.ExpandEvaluationPaths(map[string]any{"vars": "{{ index . .key }}"}, [][]string{{"vars"}}))
}
