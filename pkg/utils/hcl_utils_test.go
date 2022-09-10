package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWriteToFileAsHcl(t *testing.T) {
	err := WriteToFileAsHcl("/dev/stdout", "", 0644)
	assert.NotNil(t, err)
}
