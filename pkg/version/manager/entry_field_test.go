package manager

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEffectiveEntryField(t *testing.T) {
	entry := &EffectiveEntry{
		Name:       "opentofu",
		Ecosystem:  "toolchain",
		Datasource: "github-releases",
		Provider:   "hashicorp",
		Package:    "opentofu/opentofu",
		Desired:    "1.10.0",
		Group:      "tools",
		Update:     schema.VersionUpdatePolicy{Strategy: "minor", Pin: "digest"},
		Include:    []string{"linux", "darwin"},
		Exclude:    []string{"windows"},
		Prerelease: true,
		Labels:     []string{"infra", "core"},
		Locked:     "1.10.0",
	}

	tests := []struct {
		field string
		want  any
	}{
		{"name", entry.Name},
		{"ecosystem", entry.Ecosystem},
		{"datasource", entry.Datasource},
		{"provider", entry.Provider},
		{"package", entry.Package},
		{"desired", entry.Desired},
		{"group", entry.Group},
		{"update", entry.Update},
		{"include", entry.Include},
		{"exclude", entry.Exclude},
		{"prerelease", entry.Prerelease},
		{"labels", entry.Labels},
		{"locked", entry.Locked},
		// Field names are matched case-insensitively.
		{"NAME", entry.Name},
		{"Locked", entry.Locked},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, err := entry.Field(tt.field)
			if err != nil {
				t.Fatalf("Field(%q) unexpected error: %v", tt.field, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Field(%q) = %#v, want %#v", tt.field, got, tt.want)
			}
		})
	}
}

func TestEffectiveEntryFieldUnsupported(t *testing.T) {
	entry := &EffectiveEntry{Name: "opentofu"}

	_, err := entry.Field("bogus")
	if !errors.Is(err, ErrUnsupportedEntryField) {
		t.Fatalf("Field(\"bogus\") error = %v, want %v", err, ErrUnsupportedEntryField)
	}
}
