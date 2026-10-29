package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCheckFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "config"), 0o700))
	for name, contents := range map[string]string{
		"a.yaml": "metadata:\n  owner: other\n", "b.json": `{"metadata":{"owner":"platform"}}`,
		"c.yml": "metadata: {}", "skip.json": "not JSON", "notes.txt": "not YAML",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "config", name), []byte(contents), 0o600))
	}
	rule := Rule{
		ID: "owner", On: File, Engine: JSONSchemaEngine, Effects: Effects{Deny: true},
		Files:  &FileSelector{Include: []string{"config/**/*", "config/*.json"}, Exclude: []string{"**/skip.json"}},
		Schema: &SchemaConfig{Entry: schemaBase + "schema.json", Resources: map[string]json.RawMessage{schemaBase + "schema.json": json.RawMessage(optionalOwnerSchema)}},
	}
	results, err := CheckFiles(t.Context(), &Bundle{Rules: []Rule{rule}}, root)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Len(t, results, 3)
	assert.Equal(t, "config/a.yaml", results[0].File)
	assert.Equal(t, Deny, results[0].Decision.Status)
	assert.Equal(t, "/metadata/owner", results[0].Decision.Findings[0].Diagnostics[0].Path)
	assert.Equal(t, NotApplicable, results[1].Decision.Status)
	assert.Equal(t, NotApplicable, results[2].Decision.Status)
	// A successful check cannot silently hide a typo in a glob.
	rule.Files.Include = []string{"missing/**/*.yaml"}
	_, err = CheckFiles(t.Context(), &Bundle{Rules: []Rule{rule}}, root)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
}

func TestParseFileDocuments(t *testing.T) {
	for _, tc := range []struct{ name, contents, expected string }{
		{"object.yaml", "owner: platform", `{"owner":"platform"}`},
		{"array.yml", "- a\n- b\n", `["a","b"]`},
		{"null.yaml", "null", `null`},
		{"array.json", `[1,2]`, `[1,2]`},
		{"integer.yaml", "id: 9007199254740993", `{"id":9007199254740993}`},
		{"template.yaml", "value: '{{ .owner }}'", `{"value":"{{ .owner }}"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := parseFileDocument(tc.name, []byte(tc.contents))
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(raw))
		})
	}
	for name, contents := range map[string]string{
		"duplicate.yaml": "owner: a\nowner: b", "multiple.yaml": "owner: a\n---\nowner: b",
		"function.yaml": "owner: !exec echo secret", "malformed.yaml": "[",
		"empty.yaml": "", "multiple.json": "{} {}", "malformed.json": "{",
		"keys.yaml": "1: value", "recursive.yaml": "x: &x {y: *x}",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseFileDocument(name, []byte(contents))
			require.Error(t, err)
		})
	}
}

func TestFileInputAllEngines(t *testing.T) {
	for _, rule := range []Rule{
		{ID: "cel", On: File, Engine: CELEngine, Expression: "input.size() > 0"},
		{ID: "starlark", On: File, Engine: StarlarkEngine, Expression: "return len(ctx.input) > 0"},
		{ID: "opa", On: File, Engine: OPAEngine, OPA: &OPAConfig{Query: "data.test.deny", Modules: map[string]string{"test.rego": "package test\ndeny = true { count(input) > 0 }"}}},
		{ID: "schema", On: File, Engine: JSONSchemaEngine, Schema: &SchemaConfig{Entry: schemaBase + "array.json", Resources: map[string]json.RawMessage{schemaBase + "array.json": json.RawMessage(`{"type":"array","maxItems":0}`)}}},
	} {
		t.Run(rule.ID, func(t *testing.T) {
			rule.Effects.Deny = true
			decision, err := NewEngine().Evaluate(t.Context(), &Request{Kind: File, Input: json.RawMessage(`[1]`)}, []Rule{rule})
			require.NoError(t, err)
			assert.Equal(t, Deny, decision.Status)
		})
	}
}

func TestFileCheckErrorsContinue(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.json"), []byte(`{`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.json"), []byte(`{}`), 0o600))
	rule := Rule{ID: "file", On: File, Engine: CELEngine, Expression: "false", Files: &FileSelector{Include: []string{"*.json"}}}
	results, err := CheckFiles(t.Context(), &Bundle{Rules: []Rule{rule}}, root)
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
	require.Len(t, results, 2)
	assert.Equal(t, EvaluationError, results[0].Decision.Status)
	assert.NotEmpty(t, results[0].Error)
	assert.Equal(t, NotApplicable, results[1].Decision.Status)
}

func TestFileResourceConfinement(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{}`), 0o600))
	if err := os.Symlink(outside, filepath.Join(root, "escape.json")); err != nil {
		t.Skip(err)
	}
	for _, name := range []string{outside, "../outside.json", "escape.json", "..\\outside.json"} {
		_, err := readLocalResource(root, name, 100)
		require.Error(t, err, name)
	}
	for _, pattern := range []string{"../*.json", "/tmp/*.json", "[", "a/../*.json", "a\\*.json"} {
		require.Error(t, validateFileSelector(File, &FileSelector{Include: []string{pattern}}))
	}
	require.Error(t, validateFileSelector(Command, &FileSelector{Include: []string{"*.json"}}))
	require.Error(t, validateFileSelector(File, nil))
}

func TestFileCheckSelectionFailures(t *testing.T) {
	root := t.TempDir()
	rule := Rule{ID: "file", On: File, Engine: CELEngine, Expression: "false", Files: &FileSelector{Include: []string{"*.json"}}}
	for _, tc := range []struct {
		name      string
		bundle    *Bundle
		directory string
		want      error
	}{
		{"nil bundle", nil, root, errUtils.ErrPolicyInvalid},
		{"missing root", &Bundle{Rules: []Rule{rule}}, filepath.Join(root, "missing"), errUtils.ErrPolicyInputInvalid},
		{"missing selector", &Bundle{Rules: []Rule{{ID: "invalid", On: File}}}, root, errUtils.ErrPolicyInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results, err := CheckFiles(t.Context(), tc.bundle, tc.directory)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, results)
		})
	}
	// No file policy means no filesystem access, even with a nonexistent root.
	results, err := CheckFiles(t.Context(), &Bundle{}, filepath.Join(root, "missing"))
	require.NoError(t, err)
	require.Empty(t, results)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	results, err = CheckFiles(ctx, &Bundle{Rules: []Rule{rule}}, root)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, results)

	_, err = matchingFiles(t.Context(), fstest.MapFS{}, &FileSelector{Include: []string{"["}})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
}

func TestFileCheckCancellationAfterSelection(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "one.json"), []byte(`{}`), 0o600))
	rule := Rule{ID: "file", On: File, Engine: CELEngine, Expression: "true", Files: &FileSelector{Include: []string{"*.json"}}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Options run after selection and before any document is evaluated.
	results, err := CheckFiles(ctx, &Bundle{Rules: []Rule{rule}}, root, func(_ *fileCheckContext) { cancel() })
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, results)
}
