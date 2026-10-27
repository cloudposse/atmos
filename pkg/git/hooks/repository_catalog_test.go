package hooks

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRepositoryCheckCatalog(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "checks", "checks.star"))
	require.NoError(t, err)
	tests := []struct{ name, catalog, want string }{
		{"valid", "checks:\n  - id: example\n    handler: yaml_syntax\n    files: '\\.yaml$'\n", ""},
		{"empty", "checks: []", "nonempty checks list"},
		{"typo", "checks:\n  - id: example\n    handler: yaml_syntax\n    excludes: fixtures/", "Invalid repository check field"},
		{"wrong type", "checks:\n  - id: example\n    handler: yaml_syntax\n    fix: 'false'", "Invalid repository check field"},
		{"missing handler", "checks:\n  - id: example", "Unknown handler"},
		{"unknown selection", "checks:\n  - id: example\n    handler: yaml_syntax\n    selection: staged", "Unknown file selection"},
		{"duplicate", "checks:\n  - {id: example, handler: yaml_syntax}\n  - {id: example, handler: yaml_syntax}", "Duplicate repository check"},
		{"bad pattern", "checks:\n  - {id: example, handler: yaml_syntax, files: '['}", "invalid regex"},
		{"non mapping", "checks: [example]", "must be a mapping"},
		{"bad id", "checks:\n  - {id: 'Bad ID', handler: yaml_syntax}", "lowercase id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "catalog.yaml"), []byte(tt.catalog), 0o600))
			task := schema.Task{Type: "script", Interpreter: "starlark", WorkingDirectory: dir, Script: string(content) + "\ncheck_specs(\"catalog.yaml\")\n"}
			cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{task}}}}
			var output bytes.Buffer
			err := Run(cfg, "pre-commit", nil, WithOutputWriters(&output, &output))
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}
