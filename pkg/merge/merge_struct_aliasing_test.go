package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeepCopyMap_NoAliasingStructsInTypedMaps verifies that structs with nested slices/maps
// inside typed maps are properly deep copied without aliasing.
// This is a regression test for the bug where map[int]SomeStruct values containing slices/maps
// were not deep copied, causing modifications to the copy to affect the original.
// Note: Uses map[int]Config (non-string keys) to trigger copyNonStringKeyMap path.
func TestDeepCopyMap_NoAliasingStructsInTypedMaps(t *testing.T) {
	// Define a struct with nested slice and map fields.
	type Config struct {
		Tags   []string
		Labels map[string]string
		Count  int
	}

	// Create a map with NON-STRING KEYS (int) and struct values containing nested collections.
	// This triggers the copyNonStringKeyMap path which uses deepCopyTypedValue.
	original := map[string]any{
		"configs": map[int]Config{
			1: {
				Tags:   []string{"production", "critical"},
				Labels: map[string]string{"env": "prod", "tier": "backend"},
				Count:  5,
			},
			2: {
				Tags:   []string{"staging", "test"},
				Labels: map[string]string{"env": "staging", "tier": "frontend"},
				Count:  3,
			},
		},
	}

	// Deep copy the map.
	copied, err := DeepCopyMap(original)
	assert.Nil(t, err)
	assert.NotNil(t, copied)

	// Get the typed maps from both original and copy.
	originalTypedMap, ok1 := original["configs"].(map[int]Config)
	copiedTypedMap, ok2 := copied["configs"].(map[int]Config)
	assert.True(t, ok1, "Original should be map[int]Config")
	assert.True(t, ok2, "Copy should be map[int]Config")

	// Verify initial values are identical.
	assert.Equal(t, originalTypedMap[1].Tags, copiedTypedMap[1].Tags)
	assert.Equal(t, originalTypedMap[1].Labels, copiedTypedMap[1].Labels)
	assert.Equal(t, originalTypedMap[1].Count, copiedTypedMap[1].Count)

	// Modify the copy's nested slice (this would modify original if aliased).
	copiedApp1 := copiedTypedMap[1]
	copiedApp1.Tags[0] = "MODIFIED"
	copiedApp1.Tags = append(copiedApp1.Tags, "NEW_TAG")
	copiedTypedMap[1] = copiedApp1

	// Modify the copy's nested map (this would modify original if aliased).
	copiedApp1 = copiedTypedMap[1]
	copiedApp1.Labels["env"] = "MODIFIED"
	copiedApp1.Labels["new_label"] = "NEW_VALUE"
	copiedTypedMap[1] = copiedApp1

	// Modify a primitive field (should always work, this is a sanity check).
	copiedApp1 = copiedTypedMap[1]
	copiedApp1.Count = 999
	copiedTypedMap[1] = copiedApp1

	// Verify the original was NOT affected (no aliasing).
	assert.Equal(t, "production", originalTypedMap[1].Tags[0], "Original slice should not be modified")
	assert.Equal(t, 2, len(originalTypedMap[1].Tags), "Original slice length should not change")
	assert.Equal(t, []string{"production", "critical"}, originalTypedMap[1].Tags, "Original slice values unchanged")

	assert.Equal(t, "prod", originalTypedMap[1].Labels["env"], "Original map value should not be modified")
	assert.Equal(t, 2, len(originalTypedMap[1].Labels), "Original map should not have new keys")
	_, hasNewLabel := originalTypedMap[1].Labels["new_label"]
	assert.False(t, hasNewLabel, "Original map should not have new key from copy")

	assert.Equal(t, 5, originalTypedMap[1].Count, "Original primitive value should not change")

	// Verify the copy was modified.
	assert.Equal(t, "MODIFIED", copiedTypedMap[1].Tags[0], "Copy slice should be modified")
	assert.Equal(t, 3, len(copiedTypedMap[1].Tags), "Copy slice should have appended element")
	assert.Contains(t, copiedTypedMap[1].Tags, "NEW_TAG", "Copy should have new tag")

	assert.Equal(t, "MODIFIED", copiedTypedMap[1].Labels["env"], "Copy map should be modified")
	assert.Equal(t, 3, len(copiedTypedMap[1].Labels), "Copy map should have new key")
	assert.Equal(t, "NEW_VALUE", copiedTypedMap[1].Labels["new_label"], "Copy should have new label")

	assert.Equal(t, 999, copiedTypedMap[1].Count, "Copy primitive value should be modified")
}

// TestDeepCopyMap_NestedStructsInStructs verifies deep copying of nested structs.
// Uses non-string keys to trigger the deep copy path that needed the struct fix.
func TestDeepCopyMap_NestedStructsInStructs(t *testing.T) {
	type Inner struct {
		Values []int
	}

	type Outer struct {
		Name  string
		Inner Inner
		Items []Inner
	}

	original := map[string]any{
		"data": map[int]Outer{
			1: {
				Name: "outer1",
				Inner: Inner{
					Values: []int{1, 2, 3},
				},
				Items: []Inner{
					{Values: []int{10, 20}},
					{Values: []int{30, 40}},
				},
			},
		},
	}

	// Deep copy the map.
	copied, err := DeepCopyMap(original)
	assert.Nil(t, err)
	assert.NotNil(t, copied)

	// Get the typed maps.
	originalTypedMap := original["data"].(map[int]Outer)
	copiedTypedMap := copied["data"].(map[int]Outer)

	// Modify the copy's nested struct slice.
	copiedKey1 := copiedTypedMap[1]
	copiedKey1.Inner.Values[0] = 999
	copiedKey1.Items[0].Values[0] = 888
	copiedTypedMap[1] = copiedKey1

	// Verify original is NOT affected.
	assert.Equal(t, 1, originalTypedMap[1].Inner.Values[0], "Original nested struct slice should not be modified")
	assert.Equal(t, 10, originalTypedMap[1].Items[0].Values[0], "Original nested slice of structs should not be modified")

	// Verify copy was modified.
	assert.Equal(t, 999, copiedTypedMap[1].Inner.Values[0], "Copy nested struct slice should be modified")
	assert.Equal(t, 888, copiedTypedMap[1].Items[0].Values[0], "Copy nested slice of structs should be modified")
}

// TestDeepCopyMap_StructsWithMaps verifies structs containing maps are properly deep copied.
func TestDeepCopyMap_StructsWithMaps(t *testing.T) {
	type ServerConfig struct {
		Name     string
		Settings map[string]int
	}

	original := map[string]any{
		"servers": map[int]ServerConfig{
			1: {
				Name:     "server1",
				Settings: map[string]int{"cpu": 4, "memory": 8},
			},
			2: {
				Name:     "server2",
				Settings: map[string]int{"cpu": 2, "memory": 4},
			},
		},
	}

	// Deep copy.
	copied, err := DeepCopyMap(original)
	assert.Nil(t, err)

	// Get typed maps.
	originalTypedMap := original["servers"].(map[int]ServerConfig)
	copiedTypedMap := copied["servers"].(map[int]ServerConfig)

	// Modify copy's nested map.
	copiedServer1 := copiedTypedMap[1]
	copiedServer1.Settings["cpu"] = 16
	copiedServer1.Settings["new_setting"] = 100
	copiedTypedMap[1] = copiedServer1

	// Verify original NOT affected.
	assert.Equal(t, 4, originalTypedMap[1].Settings["cpu"], "Original map in struct should not be modified")
	assert.Equal(t, 2, len(originalTypedMap[1].Settings), "Original should not have new keys")

	// Verify copy modified.
	assert.Equal(t, 16, copiedTypedMap[1].Settings["cpu"], "Copy should be modified")
	assert.Equal(t, 100, copiedTypedMap[1].Settings["new_setting"], "Copy should have new key")
}
