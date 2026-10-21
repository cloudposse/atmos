package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/duration"
)

func TestTimeoutUnits(t *testing.T) {
	for _, tt := range []struct {
		name    string
		section map[string]any
		want    time.Duration
	}{
		{"omitted", nil, 0},
		{"integer seconds", map[string]any{"timeout": 30}, 30 * time.Second},
		{"numeric string seconds", map[string]any{"timeout": "30"}, 30 * time.Second},
		{"fractional seconds", map[string]any{"timeout": 1.5}, 1500 * time.Millisecond},
		{"explicit minutes", map[string]any{"timeout": "30m"}, 30 * time.Minute},
		{"legacy minutes", map[string]any{"timeout_in_minutes": 30}, 30 * time.Minute},
		{"modern wins with seconds", map[string]any{"timeout": 30, "timeout_in_minutes": 30}, 30 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Timeout(tt.section)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTimeoutInvalid(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  error
	}{
		{"bad", duration.ErrInvalid},
		{0, duration.ErrNotPositive},
		{-1, duration.ErrNotPositive},
	} {
		_, err := Timeout(map[string]any{"timeout": tt.value})
		require.ErrorIs(t, err, tt.want)
	}
}

func TestRegionDefaults(t *testing.T) {
	assert.Empty(t, ResolveRegion(nil))
	legacy := map[string]any{"settings": map[string]any{"aws_cloudformation": map[string]any{"region": "us-east-1"}}}
	assert.Equal(t, "us-east-1", ResolveRegion(legacy))
	legacy["region"] = "us-west-2"
	assert.Equal(t, "us-west-2", ResolveRegion(legacy))
	legacy["region"] = ""
	assert.Empty(t, ResolveRegion(legacy), "an explicit empty region delegates to the identity or AWS SDK")
	assert.Equal(t, map[string]any{"region": "us-west-2", "timeout": 30}, DefaultsSection(map[string]any{
		"region": "us-west-2", "timeout": 30, "unrelated": "value",
	}))
}
