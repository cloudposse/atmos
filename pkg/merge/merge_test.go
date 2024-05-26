package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMergeBasic(t *testing.T) {
	cliConfig := schema.CliConfiguration{}

	map1 := map[any]any{"foo": "bar"}
	map2 := map[any]any{"baz": "bat"}

	inputs := []map[any]any{map1, map2}
	expected := map[any]any{"foo": "bar", "baz": "bat"}

	result, err := Merge(cliConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}

func TestMergeBasicOverride(t *testing.T) {
	cliConfig := schema.CliConfiguration{}

	map1 := map[any]any{"foo": "bar"}
	map2 := map[any]any{"baz": "bat"}
	map3 := map[any]any{"foo": "ood"}

	inputs := []map[any]any{map1, map2, map3}
	expected := map[any]any{"foo": "ood", "baz": "bat"}

	result, err := Merge(cliConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}

func TestMergeListReplace(t *testing.T) {
	cliConfig := schema.CliConfiguration{
		Settings: schema.CliSettings{
			ListMergeStrategy: ListMergeStrategyReplace,
		},
	}

	map1 := map[any]any{
		"list": []string{"1", "2", "3"},
	}

	map2 := map[any]any{
		"list": []string{"4", "5", "6"},
	}

	inputs := []map[any]any{map1, map2}
	expected := map[any]any{"list": []any{"4", "5", "6"}}

	result, err := Merge(cliConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)

	yamlConfig, err := yaml.Marshal(result)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))
}

func TestMergeListAppend(t *testing.T) {
	cliConfig := schema.CliConfiguration{
		Settings: schema.CliSettings{
			ListMergeStrategy: ListMergeStrategyAppend,
		},
	}

	map1 := map[any]any{
		"list": []string{"1", "2", "3"},
	}

	map2 := map[any]any{
		"list": []string{"4", "5", "6"},
	}

	inputs := []map[any]any{map1, map2}
	expected := map[any]any{"list": []any{"1", "2", "3", "4", "5", "6"}}

	result, err := Merge(cliConfig, inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)

	yamlConfig, err := yaml.Marshal(result)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))
}

func TestMergeListMerge(t *testing.T) {
	cliConfig := schema.CliConfiguration{
		Settings: schema.CliSettings{
			ListMergeStrategy: ListMergeStrategyMerge,
		},
	}

	map1 := map[any]any{
		"list": []map[string]string{
			{
				"1": "1",
				"2": "2",
				"3": "3",
				"4": "4",
			},
		},
	}

	map2 := map[any]any{
		"list": []map[string]string{
			{
				"1": "1b",
				"2": "2",
				"3": "3b",
				"5": "5",
			},
		},
	}

	inputs := []map[any]any{map1, map2}

	result, err := Merge(cliConfig, inputs)
	assert.Nil(t, err)

	var mergedList []any
	var ok bool

	if mergedList, ok = result["list"].([]any); !ok {
		t.Errorf("invalid merge result: %v", result)
	}

	merged := mergedList[0].(map[any]any)

	assert.Equal(t, "1b", merged["1"])
	assert.Equal(t, "2", merged["2"])
	assert.Equal(t, "3b", merged["3"])
	assert.Equal(t, "4", merged["4"])
	assert.Equal(t, "5", merged["5"])

	yamlConfig, err := yaml.Marshal(result)
	assert.Nil(t, err)
	t.Log(string(yamlConfig))
}
