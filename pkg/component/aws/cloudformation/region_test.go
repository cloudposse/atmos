package cloudformation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveRegion(t *testing.T) {
	assert.Equal(t, "", resolveRegion(nil))
	assert.Equal(t, "", resolveRegion(map[string]any{}))
	assert.Equal(t, "", resolveRegion(map[string]any{"settings": map[string]any{}}))

	region := resolveRegion(map[string]any{
		"settings": map[string]any{
			"aws_cloudformation": map[string]any{"region": "us-east-2"},
		},
	})
	assert.Equal(t, "us-east-2", region)
}
