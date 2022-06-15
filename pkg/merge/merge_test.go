package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeBasic(t *testing.T) {
	map1 := map[any]any{"foo": "bar"}
	map2 := map[any]any{"baz": "bat"}

	inputs := []map[any]any{map1, map2}
	expected := map[any]any{"foo": "bar", "baz": "bat"}

	result, err := Merge(inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}

func TestMergeBasicOverride(t *testing.T) {
	map1 := map[any]any{"foo": "bar"}
	map2 := map[any]any{"baz": "bat"}
	map3 := map[any]any{"foo": "ood"}

	inputs := []map[any]any{map1, map2, map3}
	expected := map[any]any{"foo": "ood", "baz": "bat"}

	result, err := Merge(inputs)
	assert.Nil(t, err)
	assert.Equal(t, expected, result)
}
