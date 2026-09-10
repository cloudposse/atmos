package column

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredSections(t *testing.T) {
	tests := []struct {
		name        string
		columns     []Config
		wantOK      bool
		wantOrdered []string // nil means "don't check contents", used only when wantOK.
	}{
		{
			name:        "stack only needs no section",
			columns:     []Config{{Name: "Stack", Value: "{{ .stack }}"}},
			wantOK:      true,
			wantOrdered: []string{},
		},
		{
			name: "stack and component need no section",
			columns: []Config{
				{Name: "Stack", Value: "{{ .stack }}"},
				{Name: "Component", Value: "{{ .component }}"},
			},
			wantOK:      true,
			wantOrdered: []string{},
		},
		{
			name:        "single vars reference",
			columns:     []Config{{Name: "Region", Value: "{{ .vars.region }}"}},
			wantOK:      true,
			wantOrdered: []string{"vars"},
		},
		{
			name: "multiple columns union sections",
			columns: []Config{
				{Name: "Region", Value: "{{ .vars.region }}"},
				{Name: "Owner", Value: "{{ .settings.owner }}"},
				{Name: "Stack", Value: "{{ .stack }}"},
			},
			wantOK:      true,
			wantOrdered: []string{"settings", "vars"},
		},
		{
			name:        "duplicate section refs dedup",
			columns:     []Config{{Name: "Both", Value: "{{ .vars.a }}{{ .vars.b }}"}},
			wantOK:      true,
			wantOrdered: []string{"vars"},
		},
		{
			name:        "metadata backed field",
			columns:     []Config{{Name: "Owner", Value: "{{ .metadata.owner }}"}},
			wantOK:      true,
			wantOrdered: []string{"metadata"},
		},
		{
			name:        "env backed field",
			columns:     []Config{{Name: "Region", Value: "{{ .env.AWS_REGION }}"}},
			wantOK:      true,
			wantOrdered: []string{"env"},
		},
		{
			name:        "backend backed field",
			columns:     []Config{{Name: "Bucket", Value: "{{ .backend.bucket }}"}},
			wantOK:      true,
			wantOrdered: []string{"backend"},
		},
		{
			name:        "derived metadata-sourced fields need no section",
			columns:     []Config{{Name: "Status", Value: "{{ .enabled }}{{ .locked }}{{ .tags }}{{ .labels }}{{ .status }}{{ .type }}"}},
			wantOK:      true,
			wantOrdered: []string{},
		},
		{
			name:    "catch-all raw triggers fallback",
			columns: []Config{{Name: "Raw", Value: "{{ .raw.anything }}"}},
			wantOK:  false,
		},
		{
			name:    "range over dynamic scope triggers fallback",
			columns: []Config{{Name: "List", Value: "{{ range .vars.list }}{{ .name }}{{ end }}"}},
			wantOK:  false,
		},
		{
			name:    "with block triggers fallback",
			columns: []Config{{Name: "With", Value: "{{ with .vars }}{{ .region }}{{ end }}"}},
			wantOK:  false,
		},
		{
			name:    "unrecognized top-level field triggers fallback",
			columns: []Config{{Name: "Mystery", Value: "{{ .some_unknown_field }}"}},
			wantOK:  false,
		},
		{
			name:    "invalid template triggers fallback",
			columns: []Config{{Name: "Bad", Value: "{{ .vars."}},
			wantOK:  false,
		},
		{
			name:        "no columns yields empty required set",
			columns:     []Config{},
			wantOK:      true,
			wantOrdered: []string{},
		},
		{
			name: "one resolvable and one unresolvable column both required to bail",
			columns: []Config{
				{Name: "Region", Value: "{{ .vars.region }}"},
				{Name: "Raw", Value: "{{ .raw }}"},
			},
			wantOK: false,
		},
		{
			// pkg/template.ExtractFieldRefs/HasDynamicScope parse with no FuncMap, so any
			// column.BuildColumnFuncMap() custom function (get, truncate, ternary, ...) fails to
			// parse there and falls back to ok=false -- the conservative behavior called out in
			// RequiredSections' doc comment ("a custom func ... that could touch anything").
			name:    "custom func from BuildColumnFuncMap triggers fallback",
			columns: []Config{{Name: "Region", Value: "{{ get .vars \"region\" }}"}},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sections, ok := RequiredSections(tt.columns)
			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				assert.Nil(t, sections)
				return
			}
			require.NotNil(t, sections, "a true result must return a non-nil slice (even when empty) -- nil is reserved for ok=false")
			sort.Strings(sections)
			assert.Equal(t, tt.wantOrdered, sections)
		})
	}
}

// TestRequiredSections_EmptyNotNil pins the nil-vs-empty contract explicitly: when ok is true and
// no section is required, the returned slice must be non-nil so downstream evaluation-gating
// (which distinguishes nil "no filter" from empty-but-non-nil "nothing required") behaves
// correctly. A regression here (e.g. returning a bare `var result []string`) would silently
// disable gating for exactly the case that matters most: default `list stacks` columns.
func TestRequiredSections_EmptyNotNil(t *testing.T) {
	sections, ok := RequiredSections([]Config{{Name: "Stack", Value: "{{ .stack }}"}})
	require.True(t, ok)
	require.NotNil(t, sections)
	assert.Empty(t, sections)
}
