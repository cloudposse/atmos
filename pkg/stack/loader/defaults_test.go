package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportedExtensions(t *testing.T) {
	exts := SupportedExtensions()

	assert.Contains(t, exts, ".yaml")
	assert.Contains(t, exts, ".yml")
	assert.Contains(t, exts, ".json")
	assert.Contains(t, exts, ".hcl")
	assert.Contains(t, exts, ".tf")
}
