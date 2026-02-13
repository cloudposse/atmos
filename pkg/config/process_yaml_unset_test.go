package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// loadYAMLIntoViper loads YAML content into a Viper instance.
// This simulates what Viper does when reading a config file.
func loadYAMLIntoViper(t *testing.T, v *viper.Viper, yamlContent string) {
	t.Helper()
	v.SetConfigType("yaml")
	err := v.ReadConfig(strings.NewReader(yamlContent))
	assert.NoError(t, err)
}

func TestPreprocessUnsetYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		checkKey    string
		shouldExist bool
	}{
		{
			name: "simple unset",
			yaml: `key1: value1
key2: !unset
key3: value3`,
			checkKey:    "key2",
			shouldExist: false,
		},
		{
			name: "nested unset",
			yaml: `parent:
  child1: value1
  child2: !unset
  child3: value3`,
			checkKey:    "parent.child2",
			shouldExist: false,
		},
		{
			name: "preserve other keys",
			yaml: `key1: value1
key2: !unset
key3: value3`,
			checkKey:    "key1",
			shouldExist: true,
		},
		{
			name: "multiple unsets",
			yaml: `settings:
  feature1: enabled
  feature2: !unset
  feature3: disabled
  feature4: !unset`,
			checkKey:    "settings.feature2",
			shouldExist: false,
		},
		{
			name: "unset in list context",
			yaml: `config:
  items:
    - item1
    - item2
  remove_this: !unset
  keep_this: value`,
			checkKey:    "config.remove_this",
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			// First load the YAML normally (this is what Viper does with ReadConfig).
			loadYAMLIntoViper(t, v, tt.yaml)
			// Then preprocess the YAML functions (including !unset).
			err := preprocessAtmosYamlFunc([]byte(tt.yaml), v)
			assert.NoError(t, err)

			// Check if the key exists (or doesn't exist) as expected.
			value := v.Get(tt.checkKey)
			if tt.shouldExist {
				assert.NotNil(t, value, "Key %s should exist", tt.checkKey)
			} else {
				assert.Nil(t, value, "Key %s should not exist (should be unset)", tt.checkKey)
			}
		})
	}
}

func TestPreprocessUnsetWithOtherFunctions(t *testing.T) {
	// Test that !unset works alongside other YAML functions.
	yamlContent := `env_var: !env HOME
exec_result: !exec echo "test"
unset_key: !unset
normal_key: normal_value`

	v := viper.New()
	// First load the YAML normally.
	loadYAMLIntoViper(t, v, yamlContent)
	// Then preprocess the YAML functions.
	err := preprocessAtmosYamlFunc([]byte(yamlContent), v)
	assert.NoError(t, err)

	// The unset key should not exist.
	assert.Nil(t, v.Get("unset_key"))
	assert.False(t, v.IsSet("unset_key"))

	// Other keys should exist.
	assert.NotNil(t, v.Get("normal_key"))
	assert.Equal(t, "normal_value", v.Get("normal_key"))

	// ENV and EXEC functions should have been processed.
	assert.NotNil(t, v.Get("env_var"))
	assert.NotNil(t, v.Get("exec_result"))
}

func TestPreprocessUnsetDeepNesting(t *testing.T) {
	yamlContent := `level1:
  level2:
    level3:
      level4:
        keep: value
        remove: !unset
        also_keep: another_value`

	v := viper.New()
	// First load the YAML normally.
	loadYAMLIntoViper(t, v, yamlContent)
	// Then preprocess the YAML functions.
	err := preprocessAtmosYamlFunc([]byte(yamlContent), v)
	assert.NoError(t, err)

	// Check the deeply nested unset.
	assert.Nil(t, v.Get("level1.level2.level3.level4.remove"))
	assert.False(t, v.IsSet("level1.level2.level3.level4.remove"))
	assert.Equal(t, "value", v.Get("level1.level2.level3.level4.keep"))
	assert.Equal(t, "another_value", v.Get("level1.level2.level3.level4.also_keep"))
}

func TestPreprocessUnsetEntireSection(t *testing.T) {
	yamlContent := `section1:
  key1: value1
  key2: value2
section2: !unset
section3:
  key3: value3`

	v := viper.New()
	// First load the YAML normally.
	loadYAMLIntoViper(t, v, yamlContent)
	// Then preprocess the YAML functions.
	err := preprocessAtmosYamlFunc([]byte(yamlContent), v)
	assert.NoError(t, err)

	// section2 should not exist.
	assert.Nil(t, v.Get("section2"))
	assert.False(t, v.IsSet("section2"))

	// Other sections should exist.
	assert.NotNil(t, v.Get("section1"))
	assert.NotNil(t, v.Get("section3"))
	assert.Equal(t, "value1", v.Get("section1.key1"))
	assert.Equal(t, "value3", v.Get("section3.key3"))
}

func TestPreprocessInvalidYAML(t *testing.T) {
	// Test that invalid YAML returns an error.
	invalidYaml := `key1: "unclosed string
key2: !unset`

	v := viper.New()
	err := preprocessAtmosYamlFunc([]byte(invalidYaml), v)
	assert.Error(t, err)
}

func TestPreprocessEmptyUnset(t *testing.T) {
	// Test !unset with no value after it.
	yamlContent := `key1: value1
key2: !unset
key3: value3`

	v := viper.New()
	// First load the YAML normally.
	loadYAMLIntoViper(t, v, yamlContent)
	// Then preprocess the YAML functions.
	err := preprocessAtmosYamlFunc([]byte(yamlContent), v)
	assert.NoError(t, err)

	assert.Nil(t, v.Get("key2"))
	assert.False(t, v.IsSet("key2"))
	assert.Equal(t, "value1", v.Get("key1"))
	assert.Equal(t, "value3", v.Get("key3"))
}

func TestDeleteNestedKey(t *testing.T) {
	// This test validates the deleteNestedKey helper function directly.
	tests := []struct {
		name       string
		initial    map[string]any
		deletePath string
		checkPath  string
		wantExists bool
	}{
		{
			name: "delete top-level key",
			initial: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			deletePath: "key1",
			checkPath:  "key1",
			wantExists: false,
		},
		{
			name: "delete nested key",
			initial: map[string]any{
				"parent": map[string]any{
					"child1": "value1",
					"child2": "value2",
				},
			},
			deletePath: "parent.child1",
			checkPath:  "parent.child1",
			wantExists: false,
		},
		{
			name: "preserve sibling keys after delete",
			initial: map[string]any{
				"parent": map[string]any{
					"child1": "value1",
					"child2": "value2",
				},
			},
			deletePath: "parent.child1",
			checkPath:  "parent.child2",
			wantExists: true,
		},
		{
			name: "delete deeply nested key",
			initial: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": map[string]any{
							"target": "delete_me",
							"keep":   "keep_me",
						},
					},
				},
			},
			deletePath: "a.b.c.target",
			checkPath:  "a.b.c.target",
			wantExists: false,
		},
		{
			name: "delete non-existent key is no-op",
			initial: map[string]any{
				"key1": "value1",
			},
			deletePath: "nonexistent",
			checkPath:  "key1",
			wantExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the deleteNestedKey helper directly.
			m := make(map[string]any)
			for k, v := range tt.initial {
				m[k] = v
			}

			segments := strings.Split(tt.deletePath, ".")
			deleteNestedKey(m, segments)

			// Check path in the map.
			checkSegments := strings.Split(tt.checkPath, ".")
			exists := nestedKeyExists(m, checkSegments)

			if tt.wantExists {
				assert.True(t, exists, "Key %s should exist", tt.checkPath)
			} else {
				assert.False(t, exists, "Key %s should not exist", tt.checkPath)
			}
		})
	}
}

// nestedKeyExists checks if a key exists in a nested map.
func nestedKeyExists(m map[string]any, segments []string) bool {
	if len(segments) == 0 {
		return false
	}

	current := m
	for i, seg := range segments {
		key := strings.ToLower(seg)
		val, ok := current[key]
		if !ok {
			return false
		}
		if i == len(segments)-1 {
			return true
		}
		nextMap, ok := val.(map[string]any)
		if !ok {
			return false
		}
		current = nextMap
	}
	return false
}

func TestUnsetKeyNotInAllSettings(t *testing.T) {
	// This test verifies that !unset truly removes a key from Viper
	// during preprocessing. The key should not appear in AllSettings().
	yamlContent := `keep: value
remove: !unset
also_keep: another_value`

	v := viper.New()
	// First load the YAML normally.
	loadYAMLIntoViper(t, v, yamlContent)
	// Then preprocess the YAML functions.
	err := preprocessAtmosYamlFunc([]byte(yamlContent), v)
	assert.NoError(t, err)

	allSettings := v.AllSettings()

	// The "remove" key should not be in AllSettings at all.
	_, exists := allSettings["remove"]
	assert.False(t, exists, "Key 'remove' should not exist in AllSettings()")

	// IsSet should return false for the removed key.
	assert.False(t, v.IsSet("remove"), "IsSet('remove') should be false")

	// Other keys should still exist.
	assert.True(t, v.IsSet("keep"), "IsSet('keep') should be true")
	assert.True(t, v.IsSet("also_keep"), "IsSet('also_keep') should be true")
}

func TestDeleteViperKeyRemovesExistingKey(t *testing.T) {
	// This test verifies that deleteViperKey truly removes a key from Viper
	// that was previously loaded via ReadConfig (which is how Atmos loads config).
	yamlContent := `parent:
  keep: keep_value
  remove: remove_value
sibling: sibling_value`

	v := viper.New()
	// Load via ReadConfig (simulates real Atmos behavior).
	loadYAMLIntoViper(t, v, yamlContent)

	// Verify all keys are initially set.
	assert.True(t, v.IsSet("parent.remove"), "parent.remove should be set initially")
	assert.True(t, v.IsSet("parent.keep"), "parent.keep should be set initially")
	assert.True(t, v.IsSet("sibling"), "sibling should be set initially")

	// Delete the target key.
	deleteViperKey(v, "parent.remove")

	// Verify the key is truly removed.
	assert.False(t, v.IsSet("parent.remove"), "parent.remove should NOT be set after deleteViperKey")
	assert.Nil(t, v.Get("parent.remove"), "parent.remove should return nil after deleteViperKey")

	// The key should not appear in AllSettings.
	allSettings := v.AllSettings()
	parentMap, ok := allSettings["parent"].(map[string]any)
	assert.True(t, ok, "parent should still exist as a map")
	_, exists := parentMap["remove"]
	assert.False(t, exists, "remove key should not exist in parent map")

	// Other keys should be preserved.
	assert.True(t, v.IsSet("parent.keep"), "parent.keep should still be set")
	assert.Equal(t, "keep_value", v.Get("parent.keep"))
	assert.True(t, v.IsSet("sibling"), "sibling should still be set")
	assert.Equal(t, "sibling_value", v.Get("sibling"))
}
