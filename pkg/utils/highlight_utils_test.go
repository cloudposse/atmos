package utils

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestHighlightCodeWithConfig(t *testing.T) {
	code, err := HighlightCodeWithConfig(&schema.AtmosConfiguration{}, `{"code":"hello"}`, "json")
	assert.NoError(t, err)
	assert.Contains(t, code, "code")
	assert.Contains(t, code, "hello")
}
