package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/http/proxy"
)

func TestFormatSavingsReport(t *testing.T) {
	tests := []struct {
		name string
		snap proxy.StatsSnapshot
		want string
		ok   bool
	}{
		{
			name: "no traffic prints nothing",
			snap: proxy.StatsSnapshot{},
			want: "",
			ok:   false,
		},
		{
			name: "cold run reports downloaded and cached",
			snap: proxy.StatsSnapshot{Misses: 1, ObjectsCached: 1, BytesCached: 1024},
			want: "Registry cache: 1.0 kB downloaded and cached (1 object)",
			ok:   true,
		},
		{
			name: "warm run reports saved",
			snap: proxy.StatsSnapshot{Hits: 3, BytesSaved: 2048},
			want: "Registry cache: 2.0 kB saved (3 hits)",
			ok:   true,
		},
		{
			name: "mixed run reports both saved and cached",
			snap: proxy.StatsSnapshot{Hits: 2, BytesSaved: 2048, Misses: 1, ObjectsCached: 1, BytesCached: 512},
			want: "Registry cache: 2.0 kB saved (2 hits); 512 B downloaded and cached (1 object)",
			ok:   true,
		},
		{
			name: "non-cacheable misses alone print nothing",
			snap: proxy.StatsSnapshot{Misses: 2},
			want: "",
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := formatSavingsReport(tt.snap)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
