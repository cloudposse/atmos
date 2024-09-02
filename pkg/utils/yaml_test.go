package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestYAMLToMapOfInterfaces(t *testing.T) {
	input := `---
hello: world`
	result, err := UnmarshalYAML[map[any]any](input)
	assert.Nil(t, err)
	assert.Equal(t, result["hello"], "world")
}

func TestYAMLToMapOfInterfacesRedPath(t *testing.T) {
	input := "Not YAML"
	_, err := UnmarshalYAML[map[any]any](input)
	assert.NotNil(t, err)
}
