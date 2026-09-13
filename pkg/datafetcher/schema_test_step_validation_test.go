package datafetcher

import "testing"

func TestManifestSchemaTestSteps(t *testing.T) {
	schema := loadEmbeddedSchemaBytes(t)
	for _, output := range []string{"failures", "all", "raw"} {
		t.Run(output, func(t *testing.T) {
			check := map[string]any{"type": "test", "output": output, "steps": []any{map[string]any{"type": "http", "url": "https://example.com/health"}}}
			manifest := map[string]any{"workflows": map[string]any{"checks": map[string]any{"steps": []any{check}}}}
			if output == "raw" {
				assertSchemaInvalid(t, schema, manifest)
			} else {
				assertSchemaValid(t, schema, manifest)
			}
		})
	}
}

func TestManifestSchemaViewportPadding(t *testing.T) {
	schema := loadEmbeddedSchemaBytes(t)
	for _, padding := range []int{0, 2, -1} {
		step := map[string]any{"type": "shell", "command": "echo hello", "output": "viewport", "viewport": map[string]any{"height": 4, "padding": padding}}
		manifest := map[string]any{"workflows": map[string]any{"build": map[string]any{"steps": []any{step}}}}
		if padding < 0 {
			assertSchemaInvalid(t, schema, manifest)
		} else {
			assertSchemaValid(t, schema, manifest)
		}
	}
}

func TestManifestSchemaTestStepStructure(t *testing.T) {
	schema := loadEmbeddedSchemaBytes(t)
	for _, tc := range []struct {
		name  string
		check map[string]any
	}{
		{"omitted steps", map[string]any{"type": "test"}},
		{"empty steps", map[string]any{"type": "test", "steps": []any{}}},
		{"concurrency belongs on nested groups", map[string]any{"type": "test", "max_concurrency": 1, "steps": []any{map[string]any{"type": "shell", "command": "echo hello"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := map[string]any{"workflows": map[string]any{"checks": map[string]any{"steps": []any{tc.check}}}}
			assertSchemaInvalid(t, schema, manifest)
		})
	}
}
