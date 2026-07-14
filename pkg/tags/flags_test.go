package tags

import "testing"

func TestParseTagsFlag(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string returns nil", "", nil},
		{"single tag", "production", []string{"production"}},
		{"comma list is split and trimmed", "production, tier-1 , admin", []string{"production", "tier-1", "admin"}},
		{"blank entries are dropped", "production,,tier-1", []string{"production", "tier-1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTagsFlag(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseTagsFlag(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseTagsFlag(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseLabelsFlag(t *testing.T) {
	t.Run("empty string returns nil", func(t *testing.T) {
		got, err := ParseLabelsFlag("")
		if err != nil || got != nil {
			t.Fatalf("ParseLabelsFlag(\"\") = %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("multiple pairs are split and trimmed", func(t *testing.T) {
		got, err := ParseLabelsFlag("cost-center=platform, compliance = sox")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"cost-center": "platform", "compliance": "sox"}
		if len(got) != len(want) {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("blank segments between commas are skipped", func(t *testing.T) {
		got, err := ParseLabelsFlag("cost-center=platform,,compliance=sox")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"cost-center": "platform", "compliance": "sox"}
		if len(got) != len(want) {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("value containing an additional equals sign", func(t *testing.T) {
		got, err := ParseLabelsFlag("key=val=ue")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"key": "val=ue"}
		if len(got) != len(want) {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("missing equals sign errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag("cost-center"); err == nil {
			t.Fatal("expected error for missing '='")
		}
	})

	t.Run("empty key errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag("=platform"); err == nil {
			t.Fatal("expected error for empty key")
		}
	})
}
