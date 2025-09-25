package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

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
			err := preprocessAtmosYamlFunc([]byte(tt.yaml), v)
			assert.NoError(t, err)

			// Check if the key exists (or doesn't exist) as expected
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
	// Test that !unset works alongside other YAML functions
	yaml := `env_var: !env HOME
exec_result: !exec echo "test"
unset_key: !unset
normal_key: normal_value`

	v := viper.New()
	err := preprocessAtmosYamlFunc([]byte(yaml), v)
	assert.NoError(t, err)

	// The unset key should not exist
	assert.Nil(t, v.Get("unset_key"))

	// Other keys should exist
	assert.NotNil(t, v.Get("normal_key"))
	assert.Equal(t, "normal_value", v.Get("normal_key"))

	// ENV and EXEC functions should have been processed
	assert.NotNil(t, v.Get("env_var"))
	assert.NotNil(t, v.Get("exec_result"))
}

func TestPreprocessUnsetDeepNesting(t *testing.T) {
	yaml := `level1:
  level2:
    level3:
      level4:
        keep: value
        remove: !unset
        also_keep: another_value`

	v := viper.New()
	err := preprocessAtmosYamlFunc([]byte(yaml), v)
	assert.NoError(t, err)

	// Debug: print all keys
	allKeys := v.AllKeys()
	t.Logf("All keys: %v", allKeys)

	// Debug: print what we're getting for each path
	t.Logf("level1.level2.level3.level4.keep: %v", v.Get("level1.level2.level3.level4.keep"))
	t.Logf("level1.level2.level3.level4.remove: %v", v.Get("level1.level2.level3.level4.remove"))
	t.Logf("level1.level2.level3.level4.also_keep: %v", v.Get("level1.level2.level3.level4.also_keep"))

	// Check the deeply nested unset
	assert.Nil(t, v.Get("level1.level2.level3.level4.remove"))
	assert.Equal(t, "value", v.Get("level1.level2.level3.level4.keep"))
	assert.Equal(t, "another_value", v.Get("level1.level2.level3.level4.also_keep"))
}

func TestPreprocessUnsetEntireSection(t *testing.T) {
	yaml := `section1:
  key1: value1
  key2: value2
section2: !unset
section3:
  key3: value3`

	v := viper.New()
	err := preprocessAtmosYamlFunc([]byte(yaml), v)
	assert.NoError(t, err)

	// section2 should not exist
	assert.Nil(t, v.Get("section2"))

	// Other sections should exist
	assert.NotNil(t, v.Get("section1"))
	assert.NotNil(t, v.Get("section3"))
	assert.Equal(t, "value1", v.Get("section1.key1"))
	assert.Equal(t, "value3", v.Get("section3.key3"))
}

func TestPreprocessInvalidYAML(t *testing.T) {
	// Test that invalid YAML returns an error
	invalidYaml := `key1: value1
  invalid indentation here
key2: !unset`

	v := viper.New()
	err := preprocessAtmosYamlFunc([]byte(invalidYaml), v)
	assert.Error(t, err)
}

func TestPreprocessEmptyUnset(t *testing.T) {
	// Test !unset with no value after it
	yaml := `key1: value1
key2: !unset
key3: value3`

	v := viper.New()
	err := preprocessAtmosYamlFunc([]byte(yaml), v)
	assert.NoError(t, err)

	assert.Nil(t, v.Get("key2"))
	assert.Equal(t, "value1", v.Get("key1"))
	assert.Equal(t, "value3", v.Get("key3"))
}
