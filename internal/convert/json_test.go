package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONToMapOfInterfaces(t *testing.T) {
	input := "{\"hello\": \"world\"}"
	result, err := JSONToMapOfInterfaces(input)
	assert.Nil(t, err)
	assert.Equal(t, result["hello"], "world")
}

func TestJSONToMapOfInterfacesRedPath(t *testing.T) {
	input := "Not JSON"
	_, err := JSONToMapOfInterfaces(input)
	assert.NotNil(t, err)
}
