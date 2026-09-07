package tags

import "testing"

func TestMatchesTags(t *testing.T) {
	tests := []struct {
		name       string
		tags       []string
		filterTags []string
		mode       TagMode
		want       bool
	}{
		{"empty filter matches all", []string{"a"}, nil, TagModeAny, true},
		{"empty filter matches entity with no tags", nil, nil, TagModeAny, true},
		{"any: single overlap", []string{"a", "b"}, []string{"b", "c"}, TagModeAny, true},
		{"any: no overlap", []string{"a", "b"}, []string{"x", "y"}, TagModeAny, false},
		{"any: entity has no tags", nil, []string{"x"}, TagModeAny, false},
		{"all: full overlap", []string{"a", "b", "c"}, []string{"a", "b"}, TagModeAll, true},
		{"all: partial overlap", []string{"a", "b"}, []string{"a", "c"}, TagModeAll, false},
		{"all: no overlap", []string{"a"}, []string{"x", "y"}, TagModeAll, false},
		{"all: entity has no tags", nil, []string{"x"}, TagModeAll, false},
		{"any: duplicate filter tags", []string{"a"}, []string{"a", "a"}, TagModeAny, true},
		{"all: empty filter matches tagged entity", []string{"a"}, nil, TagModeAll, true},
		{"all: empty filter matches untagged entity", nil, nil, TagModeAll, true},
		{"all: duplicate filter tags with full overlap", []string{"a", "b"}, []string{"a", "a", "b"}, TagModeAll, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesTags(tt.tags, tt.filterTags, tt.mode)
			if got != tt.want {
				t.Errorf("MatchesTags(%v, %v, %v) = %v, want %v", tt.tags, tt.filterTags, tt.mode, got, tt.want)
			}
		})
	}
}
