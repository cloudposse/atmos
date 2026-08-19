package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetKeyPrefix(t *testing.T) {
	tests := []struct {
		name           string
		prefix         string
		stackDelimiter string
		stack          string
		component      string
		finalDelimiter string
		want           string
	}{
		{
			name:           "stack and component",
			prefix:         "atmos",
			stackDelimiter: "-",
			stack:          "prod-ue1",
			component:      "vpc",
			finalDelimiter: "/",
			want:           "atmos/prod/ue1/vpc",
		},
		{
			name:           "stack only",
			prefix:         "atmos",
			stackDelimiter: "-",
			stack:          "prod",
			component:      "",
			finalDelimiter: "/",
			want:           "atmos/prod",
		},
		{
			name:           "global (no stack or component)",
			prefix:         "atmos",
			stackDelimiter: "-",
			stack:          "",
			component:      "",
			finalDelimiter: "/",
			want:           "atmos",
		},
		{
			name:           "component with nested path segments",
			prefix:         "atmos",
			stackDelimiter: "-",
			stack:          "prod",
			component:      "network/vpc",
			finalDelimiter: "/",
			want:           "atmos/prod/network/vpc",
		},
		{
			name:           "hyphen final delimiter (e.g. Azure Key Vault)",
			prefix:         "atmos",
			stackDelimiter: "-",
			stack:          "prod-ue1",
			component:      "vpc",
			finalDelimiter: "-",
			want:           "atmos-prod-ue1-vpc",
		},
		{
			name:           "empty prefix",
			prefix:         "",
			stackDelimiter: "-",
			stack:          "prod",
			component:      "vpc",
			finalDelimiter: "/",
			want:           "/prod/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getKeyPrefix(tt.prefix, tt.stackDelimiter, tt.stack, tt.component, tt.finalDelimiter)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestGetKeyPrefix_MatchesGetKey verifies getKeyPrefix produces exactly the prefix getKey
// composes a full key under. Every key getKey builds starts with getKeyPrefix's result plus
// finalDelimiter, so ListableStore implementations that use getKeyPrefix to scope a listing call
// stay consistent with Get/Set's own key composition.
func TestGetKeyPrefix_MatchesGetKey(t *testing.T) {
	prefix := getKeyPrefix("atmos", "-", "prod-ue1", "vpc", "/")
	fullKey, err := getKey("atmos", "-", "prod-ue1", "vpc", "image_tag", "/")
	assert.NoError(t, err)
	assert.Equal(t, prefix+"/image_tag", fullKey)
}
