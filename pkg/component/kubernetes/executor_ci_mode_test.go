package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKubernetesCIModeEnabled(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	assert.True(t, kubernetesCIModeEnabled(map[string]any{"ci": true}))
	assert.False(t, kubernetesCIModeEnabled(map[string]any{"ci": false}))
	assert.False(t, kubernetesCIModeEnabled(map[string]any{}))
	assert.False(t, kubernetesCIModeEnabled(nil))

	t.Setenv("ATMOS_CI", "1")
	assert.True(t, kubernetesCIModeEnabled(map[string]any{}))

	t.Setenv("ATMOS_CI", "false")
	t.Setenv("CI", "yes")
	assert.True(t, kubernetesCIModeEnabled(map[string]any{}))

	t.Setenv("CI", "0")
	assert.False(t, kubernetesCIModeEnabled(map[string]any{}))
}
