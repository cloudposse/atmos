package merge

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestMergeBasic(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}

	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"baz": "bat"}

	inputs := []map[string]any{map1, map2}
	expected := map[string]any{"foo": "bar", "baz": "bat"}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}

func TestMerge_NilAtmosConfigReturnsError(t *testing.T) {
	// Nil atmosConfig should return an error to prevent panic
	map1 := map[string]any{"list": []string{"1"}}
	map2 := map[string]any{"list": []string{"2"}}
	inputs := []map[string]any{map1, map2}

	res, err := Merge(nil, inputs)
	assert.Nil(t, res)
	assert.NotNil(t, err)

	// Verify the error is properly wrapped
	assert.True(t, errors.Is(err, errUtils.ErrMerge), "Error should be wrapped with ErrMerge")
	// ErrAtmosConfigIsNil is now embedded as a string, not wrapped
	assert.Contains(t, err.Error(), "atmos config is nil")
}

func TestMergeBasicOverride(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}

	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"baz": "bat"}
	map3 := map[string]any{"foo": "ood"}

	inputs := []map[string]any{map1, map2, map3}
	expected := map[string]any{"foo": "ood", "baz": "bat"}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}

func TestMergeListReplace(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	map1 := map[string]any{
		"list": []string{"1", "2", "3"},
	}

	map2 := map[string]any{
		"list": []string{"4", "5", "6"},
	}

	inputs := []map[string]any{map1, map2}
	expected := map[string]any{"list": []any{"4", "5", "6"}}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)

	yamlConfig, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}

func TestMergeListAppend(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyAppend,
		},
	}

	map1 := map[string]any{
		"list": []string{"1", "2", "3"},
	}

	map2 := map[string]any{
		"list": []string{"4", "5", "6"},
	}

	inputs := []map[string]any{map1, map2}
	expected := map[string]any{"list": []any{"1", "2", "3", "4", "5", "6"}}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)

	yamlConfig, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}

func TestMergeListMerge(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyMerge,
		},
	}

	map1 := map[string]any{
		"list": []map[string]string{
			{
				"1": "1",
				"2": "2",
				"3": "3",
				"4": "4",
			},
		},
	}

	map2 := map[string]any{
		"list": []map[string]string{
			{
				"1": "1b",
				"2": "2",
				"3": "3b",
				"5": "5",
			},
		},
	}

	inputs := []map[string]any{map1, map2}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)

	var mergedList []any
	var ok bool

	if mergedList, ok = result["list"].([]any); !ok {
		t.Errorf("invalid merge result: %v", result)
	}

	merged := mergedList[0].(map[string]any)

	assert.Equal(t, "1b", merged["1"])
	assert.Equal(t, "2", merged["2"])
	assert.Equal(t, "3b", merged["3"])
	assert.Equal(t, "4", merged["4"])
	assert.Equal(t, "5", merged["5"])

	yamlConfig, err := u.ConvertToYAML(result)
	assert.Nil(t, err)
	t.Log(yamlConfig)
}

func TestMergeWithNilConfig(t *testing.T) {
	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"foo": "baz", "hello": "world"}
	inputs := []map[string]any{map1, map2}

	// Nil config should return an error
	result, err := Merge(nil, inputs)
	assert.NotNil(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "atmos config is nil")

	// Verify proper error wrapping
	assert.True(t, errors.Is(err, errUtils.ErrMerge))
	// ErrAtmosConfigIsNil is now embedded as a string, not wrapped
}

func TestMergeWithInvalidStrategy(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "invalid-strategy",
		},
	}

	map1 := map[string]any{"foo": "bar"}
	map2 := map[string]any{"foo": "baz"}
	inputs := []map[string]any{map1, map2}

	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid list merge strategy")
	assert.Contains(t, err.Error(), "invalid-strategy")
	assert.Contains(t, err.Error(), "replace, append, merge")

	// Verify error wrapping - should be wrapped with ErrMerge
	assert.True(t, errors.Is(err, errUtils.ErrMerge), "Error should be wrapped with ErrMerge")
	// ErrInvalidListMergeStrategy is now embedded in the error message, not wrapped
}

func TestMergeWithEmptyInputs(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	// Test with empty inputs slice
	inputs := []map[string]any{}
	result, err := Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)

	// Test with nil maps in inputs
	inputs = []map[string]any{nil, nil}
	result, err = Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)

	// Test with mix of empty and non-empty maps
	inputs = []map[string]any{{}, {"foo": "bar"}, {}}
	result, err = Merge(&atmosConfig, inputs)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "bar", result["foo"])
}

func TestMergeHandlesNilConfigWithoutPanic(t *testing.T) {
	// This test verifies that Merge handles nil config gracefully
	// Without the nil check in Merge, this test would panic when
	// the function tries to access atmosConfig.Settings.ListMergeStrategy

	inputs := []map[string]any{
		{"key1": "value1"},
		{"key2": "value2"},
	}

	// Call Merge with nil config - this would panic without our fix
	// at the line: if atmosConfig.Settings.ListMergeStrategy != ""
	result, err := Merge(nil, inputs)

	// Verify it returns an error instead of panicking
	assert.Nil(t, result)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "atmos config is nil")
	assert.True(t, errors.Is(err, errUtils.ErrMerge))
}
