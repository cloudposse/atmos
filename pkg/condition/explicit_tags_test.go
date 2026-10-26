package condition

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestExplicitCELSyntax(t *testing.T) {
	for _, input := range []string{`when: !cel "ci"`, `when: "ci"`, `when: !cel 'env.TEST == "yes"'`, `when: 'env.TEST == "yes"'`, `when: {all: [ci, !cel 'env.TEST == "yes"']}`} {
		var config conditionConfig
		require.NoError(t, yaml.Unmarshal([]byte(input), &config), input)
		require.True(t, config.When.Evaluate(Context{CI: true, Env: map[string]string{"TEST": "yes"}}))
		encoded, err := yaml.Marshal(config)
		require.NoError(t, err)
		var decoded conditionConfig
		require.NoError(t, yaml.Unmarshal(encoded, &decoded))
		encoded, err = json.Marshal(config)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, &decoded))
	}
	for _, input := range []string{`when: "!cel ci"`, `when: '!cel ci'`, `when: {all: [ci, '!cel ci']}`} {
		var config conditionConfig
		err := yaml.Unmarshal([]byte(input), &config)
		require.ErrorIs(t, err, ErrInvalidWhenCondition)
		require.Contains(t, err.Error(), "quoted !cel is literal text")
	}
	legacy, err := NewWithPolicy("!cel ci", false)
	require.NoError(t, err)
	require.True(t, legacy.Evaluate(Context{CI: true}))
}

func TestExplicitCELTagRetainsExpressionIdentity(t *testing.T) {
	var config conditionConfig
	require.NoError(t, yaml.Unmarshal([]byte("when: local"), &config))
	require.Error(t, yaml.Unmarshal([]byte("when: !cel local"), &config), "an explicit CEL tag must compile an expression, not select a named predicate")
	require.ErrorContains(t, json.Unmarshal([]byte(`{"when":"!cel ci"}`), &config), "quoted !cel")
}
