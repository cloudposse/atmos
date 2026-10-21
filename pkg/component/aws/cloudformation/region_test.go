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

func TestResolveRegionTopLevel(t *testing.T) {
	assert.Equal(t, "eu-west-1", resolveRegion(map[string]any{"region": "eu-west-1"}))
	assert.Equal(t, "eu-west-1", resolveRegion(map[string]any{
		"region":   "eu-west-1",
		"settings": map[string]any{"aws_cloudformation": map[string]any{"region": "us-east-1"}},
	}))
}

func TestResolveRegionDeprecationWarning(t *testing.T) {
	legacy := map[string]any{"settings": map[string]any{"aws_cloudformation": map[string]any{"region": "us-east-1"}}}
	out := normalizeUIOutput(captureStderr(t, func() { assert.Equal(t, "us-east-1", resolveRegion(legacy)) }))
	assert.Contains(t, out, "settings.aws_cloudformation.region is deprecated")
	legacy["region"] = "us-west-2"
	out = normalizeUIOutput(captureStderr(t, func() { assert.Equal(t, "us-west-2", resolveRegion(legacy)) }))
	assert.Contains(t, out, "deprecated")
	out = normalizeUIOutput(captureStderr(t, func() { assert.Equal(t, "us-west-2", resolveRegion(map[string]any{"region": "us-west-2"})) }))
	assert.Empty(t, out)
}
