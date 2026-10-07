package tags

import (
	"errors"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
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

// TestParseLabelsFlag verifies parsing and validation of the labels flag value.
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

	t.Run("colon separator pairs", func(t *testing.T) {
		got, err := ParseLabelsFlag("cost-center:platform, compliance : sox")
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
		got, err := ParseLabelsFlag("cost-center:platform,compliance=sox")
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
		got, err := ParseLabelsFlag("key:val=ue")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["key"] != "val=ue" {
			t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", "key", got["key"], "val=ue")
		}
	})

	t.Run("equals first splits on equals", func(t *testing.T) {
		got, err := ParseLabelsFlag("key=val:ue")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["key"] != "val:ue" {
			t.Fatalf("ParseLabelsFlag()[%q] = %q, want %q", "key", got["key"], "val:ue")
		}
	})

	t.Run("missing separator errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag("cost-center"); err == nil {
			t.Fatal("expected error for missing separator")
		}
	})

	t.Run("empty key errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag("=platform"); err == nil {
			t.Fatal("expected error for empty key")
		}
	})

	t.Run("empty key via colon errors", func(t *testing.T) {
		if _, err := ParseLabelsFlag(":platform"); err == nil {
			t.Fatal("expected error for empty key")
		}
	})

	t.Run("duplicate keys: last value wins", func(t *testing.T) {
		got, err := ParseLabelsFlag("tier=foundational,tier=edge")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"tier": "edge"}
		if len(got) != len(want) || got["tier"] != want["tier"] {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
	})

	t.Run("whitespace-only segment is skipped like an empty one", func(t *testing.T) {
		got, err := ParseLabelsFlag("  ,tier=edge")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"tier": "edge"}
		if len(got) != len(want) || got["tier"] != want["tier"] {
			t.Fatalf("ParseLabelsFlag() = %v, want %v", got, want)
		}
	})

	t.Run("malformed pair error names the pair, the default source, and carries a hint", func(t *testing.T) {
		for _, input := range []string{"ci=auto,cost-center", "=platform", ":platform"} {
			_, err := ParseLabelsFlag(input)
			if err == nil {
				t.Fatalf("ParseLabelsFlag(%q): expected error", input)
			}
			if !errors.Is(err, errUtils.ErrInvalidFlag) {
				t.Fatalf("ParseLabelsFlag(%q) error %v does not wrap ErrInvalidFlag", input, err)
			}
			if !strings.Contains(err.Error(), "for --labels") {
				t.Fatalf("ParseLabelsFlag(%q) error %q does not name the --labels source", input, err)
			}
			if hints := cockroachErrors.GetAllHints(err); len(hints) != 1 || !strings.Contains(hints[0], "key=value") {
				t.Fatalf("ParseLabelsFlag(%q) hints = %v, want one key=value hint", input, hints)
			}
		}
	})
}

// TestParseLabelsFlagFrom verifies that labels parse like ParseLabelsFlag and that errors name the given source.
func TestParseLabelsFlagFrom(t *testing.T) {
	const source = "--labels (or ATMOS_LABELS)"

	t.Run("valid input parses like ParseLabelsFlag", func(t *testing.T) {
		got, err := ParseLabelsFlagFrom("ci=auto, team:platform,ci=manual", source)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{"ci": "manual", "team": "platform"}
		if len(got) != len(want) || got["ci"] != want["ci"] || got["team"] != want["team"] {
			t.Fatalf("ParseLabelsFlagFrom() = %v, want %v (duplicate keys are last-wins)", got, want)
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		got, err := ParseLabelsFlagFrom("", source)
		if err != nil || got != nil {
			t.Fatalf("ParseLabelsFlagFrom(\"\") = %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("error contains the caller-supplied source and the offending pair", func(t *testing.T) {
		_, err := ParseLabelsFlagFrom("ci=auto,oops", source)
		if err == nil {
			t.Fatal("expected error for missing separator")
		}
		if !errors.Is(err, errUtils.ErrInvalidFlag) {
			t.Fatalf("error %v does not wrap ErrInvalidFlag", err)
		}
		for _, want := range []string{source, `"oops"`, "expected key=value or key:value"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not contain %q", err, want)
			}
		}
		hints := cockroachErrors.GetAllHints(err)
		if len(hints) != 1 || !strings.Contains(hints[0], "--labels=ci=auto,team=platform") {
			t.Fatalf("hints = %v, want one hint with an example", hints)
		}
	})

	t.Run("empty key error names the pair and source", func(t *testing.T) {
		_, err := ParseLabelsFlagFrom("=platform", source)
		if err == nil {
			t.Fatal("expected error for empty key")
		}
		if !strings.Contains(err.Error(), source) || !strings.Contains(err.Error(), `"=platform"`) {
			t.Fatalf("error %q should contain the source and the pair", err)
		}
	})
}
