package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestYAMLToMapOfInterfaces(t *testing.T) {
	input := `---
hello: world`
	result, err := YAMLToMapOfStrings(input)
	assert.Nil(t, err)
	assert.Equal(t, result["hello"], "world")
}

func TestYAMLToMapOfInterfacesRedPath(t *testing.T) {
	input := "Not YAML"
	_, err := YAMLToMapOfStrings(input)
	assert.NotNil(t, err)
}
