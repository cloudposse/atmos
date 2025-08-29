package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteCoverageSection(t *testing.T) {
	tests := []struct {
		name      string
		coverage  string
		wantEmoji string
		wantText  string
	}{
		{
			name:      "high coverage",
			coverage:  "85.5%",
			wantEmoji: "🟢",
			wantText:  "85.5%",
		},
		{
			name:      "medium coverage",
			coverage:  "65.0%",
			wantEmoji: "🟡",
			wantText:  "65.0%",
		},
		{
			name:      "low coverage",
			coverage:  "30.0%",
			wantEmoji: "🔴",
			wantText:  "30.0%",
		},
		{
			name:      "exact high threshold",
			coverage:  "80.0%",
			wantEmoji: "🟢",
			wantText:  "80.0%",
		},
		{
			name:      "exact medium threshold",
			coverage:  "40.0%",
			wantEmoji: "🟡",
			wantText:  "40.0%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeLegacyCoverageSection(&buf, tt.coverage)
			output := buf.String()

			checkContainsAll(t, output, tt.wantEmoji, tt.wantText, "Statement Coverage")
		})
	}
}

func TestShortPackage(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
		want string
	}{
		{
			name: "full github path",
			pkg:  "github.com/cloudposse/atmos/cmd",
			want: "cmd",
		},
		{
			name: "simple path",
			pkg:  "pkg/utils",
			want: "utils",
		},
		{
			name: "single component",
			pkg:  "main",
			want: "main",
		},
		{
			name: "empty string",
			pkg:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortPackage(tt.pkg)
			if got != tt.want {
				t.Errorf("shortPackage(%q) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

// checkContainsAll checks if the output contains all expected strings.
func checkContainsAll(t *testing.T, got string, want ...string) {
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("Output missing expected content: %s\nGot:\n%s", w, got)
		}
	}
}
