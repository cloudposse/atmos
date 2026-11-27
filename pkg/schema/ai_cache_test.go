package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAICacheSettings_IsEnabled(t *testing.T) {
	enabled, disabled := true, false

	var nilSettings *AICacheSettings
	assert.True(t, nilSettings.IsEnabled(), "nil settings keep caching on")
	assert.True(t, (&AICacheSettings{}).IsEnabled(), "omitted enabled keeps caching on")
	assert.True(t, (&AICacheSettings{Enabled: &enabled}).IsEnabled())
	assert.False(t, (&AICacheSettings{Enabled: &disabled}).IsEnabled())
}
