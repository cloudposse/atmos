package tags

import "testing"

func TestMatchesLabels(t *testing.T) {
	tests := []struct {
		name         string
		labels       map[string]string
		filterLabels map[string]string
		want         bool
	}{
		{"empty filter matches all", map[string]string{"a": "1"}, nil, true},
		{"empty filter matches entity with no labels", nil, nil, true},
		{"single match", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1"}, true},
		{"all pairs must match", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2"}, true},
		{"partial match fails", map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}, false},
		{"wrong value fails", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false},
		{"missing key fails", map[string]string{"a": "1"}, map[string]string{"b": "2"}, false},
		{"entity has no labels", nil, map[string]string{"a": "1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesLabels(tt.labels, tt.filterLabels)
			if got != tt.want {
				t.Errorf("MatchesLabels(%v, %v) = %v, want %v", tt.labels, tt.filterLabels, got, tt.want)
			}
		})
	}
}
