package deferred

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEvaluationDemand(t *testing.T) {
	input := map[string]any{
		"vars":     map[string]any{"name": "{{ .settings.owner }}", "unused": "!aws.account_id"},
		"settings": map[string]any{"owner": "{{ .env.TEAM }}"},
		"env":      map[string]any{"TEAM": "platform"},
	}
	paths := ExpandEvaluationPaths(input, [][]string{{"vars", "name"}})
	assert.ElementsMatch(t, [][]string{{"vars", "name"}, {"settings", "owner"}, {"env", "TEAM"}}, paths)
	selected, excluded := SplitEvaluationFields(input, paths)
	assert.NotContains(t, selected["vars"], "unused")
	RestoreEvaluationFields(selected, excluded)
	assert.Equal(t, input, selected)
	assert.Nil(t, ExpandEvaluationPaths(map[string]any{"vars": "{{ index . .key }}"}, [][]string{{"vars"}}))
	empty, _ := SplitEvaluationFields(input, [][]string{})
	assert.Empty(t, empty)
	all, _ := SplitEvaluationFields(input, nil)
	assert.Equal(t, input, all)
	assert.True(t, IsSectionRequired(nil, "vars"))
	assert.False(t, IsSectionRequired([]string{}, "vars"))
	assert.Equal(t, [][]string{{"vars", "name"}}, PathsForQuery(".vars.name"))
	assert.Nil(t, PathsForQuery(".vars | select(.enabled)"))
}

func TestAuthCacheIncludesStackAndConfiguration(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	factory := NewMockAuthFactory(gomock.NewController(t))
	ac.DeferredAuth = NewAuthResolver(AuthOptions{Factory: factory})
	failure := errors.Join(errUtils.ErrAuthenticationUnavailable, errUtils.ErrExpiredCredentials)
	for _, stack := range []string{"dev", "prod"} {
		factory.EXPECT().Create(ac, gomock.Any(), stack).Return(nil, failure).Times(2)
		for _, name := range []string{"first", "second"} {
			info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentSection: map[string]any{"auth": map[string]any{"identities": map[string]any{name: map[string]any{"kind": "aws/user", "default": true}}}}}
			for range 2 {
				require.ErrorIs(t, ResolveAuth(ac, info), errUtils.ErrAuthenticationUnavailable)
				assert.Nil(t, info.AuthContext)
				assert.Nil(t, info.AuthManager)
			}
		}
	}
}

func TestRenderValuesPreservesStructure(t *testing.T) {
	input := map[string]any{"list": []any{"first", 42}, "nested": map[string]any{"second": "second"}}
	got, err := RenderValues(input, func(s string) (any, error) { return "rendered:" + s, nil })
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"list": []any{"rendered:first", 42}, "nested": map[string]any{"second": "rendered:second"}}, got)
	_, err = RenderValues(input, func(string) (any, error) { return nil, errUtils.ErrInvalidAuthConfig })
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}
