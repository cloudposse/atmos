package tags

import "testing"

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"nil returns empty slice", nil, []string{}},
		{"wrong type returns empty slice", "not a slice", []string{}},
		{"typed strings", []any{"a", "b"}, []string{"a", "b"}},
		{"non-string elements are skipped", []any{"a", 1, "b"}, []string{"a", "b"}},
		{"empty slice", []any{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToStringSlice(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("ToStringSlice(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ToStringSlice(%v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestToStringMap(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want map[string]string
	}{
		{"nil returns empty map", nil, map[string]string{}},
		{"wrong type returns empty map", "not a map", map[string]string{}},
		{"typed strings", map[string]any{"a": "1"}, map[string]string{"a": "1"}},
		{"non-string values are skipped", map[string]any{"a": "1", "b": 2}, map[string]string{"a": "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToStringMap(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("ToStringMap(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("ToStringMap(%v)[%q] = %q, want %q", tt.in, k, got[k], v)
				}
			}
		})
	}
}

func TestSortedKeysAndValues(t *testing.T) {
	m := map[string]string{"b": "2", "a": "1", "c": "3"}

	keys := SortedKeys(m)
	wantKeys := []string{"a", "b", "c"}
	for i := range wantKeys {
		if keys[i] != wantKeys[i] {
			t.Fatalf("SortedKeys() = %v, want %v", keys, wantKeys)
		}
	}

	values := SortedValues(m)
	wantValues := []string{"1", "2", "3"}
	for i := range wantValues {
		if values[i] != wantValues[i] {
			t.Fatalf("SortedValues() = %v, want %v", values, wantValues)
		}
	}
}
