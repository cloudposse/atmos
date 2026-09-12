package tags

import (
	"testing"

	"github.com/spf13/pflag"
)

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

// TestParseLabelsFlag exercises ParseLabelsFlag with input shaped the way pflag's
// StringSlice flag type actually hands it over: comma-splitting within a single
// --labels occurrence happens in pflag itself, so a raw multi-pair-in-one-occurrence
// case is represented here as one slice element still containing the embedded comma
// (matching what pflag would pass through before ParseLabelsFlag ever sees it), while
// pairs meant to represent separate --labels occurrences are separate slice elements.
func TestParseLabelsFlag(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		got, err := ParseLabelsFlag(nil)
		if err != nil || got != nil {
			t.Fatalf("ParseLabelsFlag(nil) = %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("empty slice returns nil", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{})
		if err != nil || got != nil {
			t.Fatalf("ParseLabelsFlag([]string{}) = %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("multiple pairs within one occurrence are split and trimmed", func(t *testing.T) {
		// pflag comma-splits a single "--labels a=1, b=2" occurrence before
		// ParseLabelsFlag sees it, but does not trim whitespace around each
		// element -- so the embedded comma survives here as one slice element
		// containing both pairs (mirroring what pflag actually produces).
		got, err := ParseLabelsFlag([]string{"cost-center=platform", " compliance = sox"})
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

	t.Run("blank elements are skipped", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{"cost-center=platform", "", "compliance=sox"})
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
		got, err := ParseLabelsFlag([]string{"key=val=ue"})
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

	t.Run("colon separator pairs", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{"cost-center:platform", " compliance : sox"})
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

	t.Run("mixed equals and colon separators", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{"cost-center:platform", "compliance=sox"})
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

	t.Run("colon first splits on colon", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{"key:val=ue"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["key"] != "val=ue" {
			t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", "key", got["key"], "val=ue")
		}
	})

	t.Run("equals first splits on equals", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{"key=val:ue"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["key"] != "val:ue" {
			t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", "key", got["key"], "val:ue")
		}
	})

	t.Run("missing separator errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag([]string{"cost-center"}); err == nil {
			t.Fatal("expected error for missing separator")
		}
	})

	t.Run("empty key errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag([]string{"=platform"}); err == nil {
			t.Fatal("expected error for empty key")
		}
	})

	t.Run("empty key via colon errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag([]string{":platform"}); err == nil {
			t.Fatal("expected error for empty key")
		}
	})

	t.Run("duplicate keys across repeated occurrences: last value wins", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{"tier=foundational", "tier=edge"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"tier": "edge"}
		if len(got) != len(want) || got["tier"] != want["tier"] {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
	})

	t.Run("whitespace-only element is skipped like an empty one", func(t *testing.T) {
		got, err := ParseLabelsFlag([]string{"  ", "tier=edge"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"tier": "edge"}
		if len(got) != len(want) || got["tier"] != want["tier"] {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
	})
}

// TestParseLabelsFlag_PflagStringSliceRepeatAccumulates proves the end-to-end contract that
// motivated switching --labels from a plain string flag to a pflag StringSlice flag: repeating
// --labels on the command line now ACCUMULATES pairs from every occurrence (matching --tags'
// existing behavior) instead of the last occurrence silently overwriting all previous ones, while
// a single occurrence still accepts a comma-separated list of pairs exactly as before.
func TestParseLabelsFlag_PflagStringSliceRepeatAccumulates(t *testing.T) {
	t.Run("repeated --labels occurrences accumulate rather than overwrite", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.StringSlice("labels", nil, "")

		err := fs.Parse([]string{"--labels", "a=1", "--labels", "b=2"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}

		raw, err := fs.GetStringSlice("labels")
		if err != nil {
			t.Fatalf("unexpected error reading flag: %v", err)
		}

		got, err := ParseLabelsFlag(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"a": "1", "b": "2"}
		if len(got) != len(want) || got["a"] != want["a"] || got["b"] != want["b"] {
			t.Fatalf("ParseLabelsFlag() = %v, want %v (both occurrences must survive, not just the last)", got, want)
		}
	})

	t.Run("single occurrence still accepts a comma-separated list", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.StringSlice("labels", nil, "")

		err := fs.Parse([]string{"--labels=a=1,b=2"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}

		raw, err := fs.GetStringSlice("labels")
		if err != nil {
			t.Fatalf("unexpected error reading flag: %v", err)
		}

		got, err := ParseLabelsFlag(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"a": "1", "b": "2"}
		if len(got) != len(want) || got["a"] != want["a"] || got["b"] != want["b"] {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
	})

	t.Run("mixing repeated occurrences and comma-separated pairs within an occurrence both accumulate", func(t *testing.T) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		fs.StringSlice("labels", nil, "")

		err := fs.Parse([]string{"--labels=a=1,b=2", "--labels", "c=3"})
		if err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}

		raw, err := fs.GetStringSlice("labels")
		if err != nil {
			t.Fatalf("unexpected error reading flag: %v", err)
		}

		got, err := ParseLabelsFlag(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"a": "1", "b": "2", "c": "3"}
		if len(got) != len(want) {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", k, got[k], v)
			}
		}
	})
}
