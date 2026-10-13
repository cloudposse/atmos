package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// scriptSourceProject builds a project with base/stacks/deploy/dev.yaml and a few scripts.
func scriptSourceProject(t *testing.T) (*schema.AtmosConfiguration, string) {
	t.Helper()
	base := t.TempDir()
	files := map[string]string{
		filepath.Join("scripts", "hook.star"):           "print('hook')\n",
		filepath.Join("scripts", "hooks", "check.star"): "print('check')\n",
		filepath.Join("stacks", "deploy", "local.star"): "print('local')\n",
		filepath.Join("scripts", "data.yaml"):           "body: print('from yaml')\n",
	}
	for name, content := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(base, name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(base, name), []byte(content), 0o600))
	}
	return &schema.AtmosConfiguration{BasePath: base, BasePathAbsolute: base}, filepath.Join(base, "stacks", "deploy", "dev.yaml")
}

func decodeScriptStep(t *testing.T, cfg *schema.AtmosConfiguration, manifest, yamlText string) map[string]any {
	t.Helper()
	out, err := UnmarshalYAMLFromFile[map[string]any](cfg, yamlText, manifest)
	require.NoError(t, err)
	with, ok := out["with"].(map[string]any)
	require.True(t, ok, "with must be a mapping: %v", out)
	return with
}

func TestIncludeScriptSource_RecordsLocalInclude(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)
	outside := filepath.Join(t.TempDir(), "outside.star")
	require.NoError(t, os.WriteFile(outside, []byte("print('outside')\n"), 0o600))

	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "bare path resolves against the base path and is stored relative",
			yaml: "type: script\nwith:\n  interpreter: starlark\n  script: !include scripts/hook.star\n",
			want: "scripts/hook.star",
		},
		{
			name: "include.raw is recorded too",
			yaml: "type: script\nwith:\n  interpreter: starlark\n  script: !include.raw scripts/hooks/check.star\n",
			want: "scripts/hooks/check.star",
		},
		{
			name: "path relative to the manifest",
			yaml: "type: script\nwith:\n  interpreter: starlark\n  script: !include ./local.star\n",
			want: "stacks/deploy/local.star",
		},
		{
			name: "file outside the base path is recorded absolute",
			yaml: "type: script\nwith:\n  interpreter: starlark\n  script: !include " + outside + "\n",
			want: outside,
		},
		{
			name: "an include-derived value replaces a hand-written script_source",
			yaml: "type: script\nwith:\n  interpreter: starlark\n  script_source: hand/written.star\n  script: !include scripts/hook.star\n",
			want: "scripts/hook.star",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			with := decodeScriptStep(t, cfg, manifest, tt.yaml)
			assert.Equal(t, tt.want, with[ScriptSourceKey])
			script, ok := with["script"].(string)
			require.True(t, ok)
			assert.NotEmpty(t, script)
			assert.Equal(t, ScriptSourceHash(script), with[ScriptSourceSHA256Key])
			assert.Equal(t, tt.want, ScriptSourceMatches(with))
			assert.Equal(t, "starlark", with["interpreter"])
		})
	}
}

func TestIncludeScriptSource_StepsList(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)
	out, err := UnmarshalYAMLFromFile[map[string]any](cfg,
		"type: script\nwith:\n"+
			"  - type: script\n    interpreter: starlark\n    script: !include scripts/hook.star\n"+
			"  - type: script\n    interpreter: starlark\n    script: print('inline')\n"+
			"  - type: script\n    interpreter: starlark\n    script: !include scripts/hooks/check.star\n",
		manifest)
	require.NoError(t, err)
	steps, ok := out["with"].([]any)
	require.True(t, ok)
	require.Len(t, steps, 3)
	first, _ := steps[0].(map[string]any)
	second, _ := steps[1].(map[string]any)
	third, _ := steps[2].(map[string]any)
	assert.Equal(t, "scripts/hook.star", first[ScriptSourceKey])
	assert.NotEmpty(t, first[ScriptSourceSHA256Key])
	assert.NotContains(t, second, ScriptSourceKey)
	assert.NotContains(t, second, ScriptSourceSHA256Key)
	assert.Equal(t, "scripts/hooks/check.star", third[ScriptSourceKey])
}

func TestIncludeScriptSource_NotRecorded(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)

	// Serve a remote script through the test HTTP client.
	previous := TestHTTPClient
	TestHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("print('remote')\n")),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { TestHTTPClient = previous })

	tests := []struct {
		name string
		yaml string
	}{
		{"inline script", "type: script\nwith:\n  interpreter: starlark\n  script: print('inline')\n"},
		{"remote include", "type: script\nwith:\n  interpreter: starlark\n  script: !include https://example.com/x.star\n"},
		{"include with a yq query", "type: script\nwith:\n  interpreter: starlark\n  script: !include scripts/data.yaml .body\n"},
		{"mapping without an interpreter", "type: script\nwith:\n  script: !include scripts/hook.star\n"},
		{"mapping without a script type", "with:\n  interpreter: starlark\n  script: !include.raw scripts/hook.star\n"},
		{"payload of another hook type", "type: shell\nwith:\n  interpreter: starlark\n  script: !include.raw scripts/hook.star\n"},
		{"include under a different key", "type: script\nwith:\n  interpreter: starlark\n  command: !include scripts/hook.star\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := UnmarshalYAMLFromFile[map[string]any](cfg, tt.yaml, manifest)
			require.NoError(t, err)
			with, ok := out["with"].(map[string]any)
			require.True(t, ok)
			assert.NotContains(t, with, ScriptSourceKey)
			assert.NotContains(t, with, ScriptSourceSHA256Key)
		})
	}
}

func TestIncludeScriptSource_PreservesStackData(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)
	for _, section := range []string{"vars", "settings", "env", "metadata", "backend"} {
		t.Run(section, func(t *testing.T) {
			for _, stepType := range []string{"", "    type: script\n"} {
				input := section + ":\n  bootstrap:\n" + stepType +
					"    interpreter: starlark\n    script: !include.raw scripts/hook.star\n"
				got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, manifest)
				require.NoError(t, err)
				data := got[section].(map[string]any)["bootstrap"].(map[string]any)
				want := map[string]any{"interpreter": "starlark", "script": "print('hook')\n"}
				if stepType != "" {
					want["type"] = "script"
				}
				assert.Equal(t, want, data, "stack data must retain exactly its declared fields")
			}
		})
	}
}

func TestIncludeScriptSource_PreservesAppendedData(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)
	for _, section := range []string{"vars", "settings", "metadata"} {
		t.Run(section, func(t *testing.T) {
			input := section + ": !append\n  - type: script\n    interpreter: starlark\n    script: !include.raw scripts/hook.star\n"
			got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, manifest)
			require.NoError(t, err)
			want := map[string]any{AppendTagMetadataKey: []any{map[string]any{
				"type": "script", "interpreter": "starlark", "script": "print('hook')\n",
			}}}
			assert.Equal(t, want, got[section], "append rewriting must preserve the data-section context")
		})
	}
	t.Run("appended script steps retain provenance", func(t *testing.T) {
		input := "steps: !append\n  - type: script\n    interpreter: starlark\n    script: !include.raw scripts/hook.star\n"
		got, err := UnmarshalYAMLFromFile[map[string]any](cfg, input, manifest)
		require.NoError(t, err)
		want := map[string]any{AppendTagMetadataKey: []any{map[string]any{
			"type": "script", "interpreter": "starlark", "script": "print('hook')\n",
			ScriptSourceKey: "scripts/hook.star", ScriptSourceSHA256Key: ScriptSourceHash("print('hook')\n"),
		}}}
		assert.Equal(t, want, got["steps"])
	})
}

// The fingerprint is the hex SHA-256 of the exact bytes of the included file, for !include and
// !include.raw alike.
func TestIncludeScriptSource_HashMatchesIncludedContent(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)
	const content = "print('hook')\n"
	want := sha256.Sum256([]byte(content))

	for _, tag := range []string{"!include", "!include.raw"} {
		t.Run(tag, func(t *testing.T) {
			with := decodeScriptStep(t, cfg, manifest, "type: script\nwith:\n  interpreter: starlark\n  script: "+tag+" scripts/hook.star\n")
			assert.Equal(t, content, with["script"])
			assert.Equal(t, hex.EncodeToString(want[:]), with[ScriptSourceSHA256Key])
		})
	}
}

// Hand-written provenance is dropped when the include has no single local source, so a step never
// carries keys the loader did not record.
func TestIncludeScriptSource_RemovesHandWrittenKeysForRemoteInclude(t *testing.T) {
	cfg, manifest := scriptSourceProject(t)
	with := decodeScriptStep(t, cfg, manifest,
		"type: script\nwith:\n  interpreter: starlark\n  script_source: hand/written.star\n  script_source_sha256: abc\n"+
			"  script: !include scripts/data.yaml .body\n")
	assert.NotContains(t, with, ScriptSourceKey)
	assert.NotContains(t, with, ScriptSourceSHA256Key)
}

func TestScriptSourceMatches(t *testing.T) {
	script := "print('a')\n"
	valid := map[string]any{"script": script, ScriptSourceKey: "scripts/a.star", ScriptSourceSHA256Key: ScriptSourceHash(script)}

	tests := []struct {
		name    string
		section map[string]any
		want    string
	}{
		{"matching fingerprint", valid, "scripts/a.star"},
		{"script replaced by a child override", map[string]any{"script": "print('b')\n", ScriptSourceKey: "scripts/a.star", ScriptSourceSHA256Key: ScriptSourceHash(script)}, ""},
		{"missing fingerprint", map[string]any{"script": script, ScriptSourceKey: "scripts/a.star"}, ""},
		{"missing source", map[string]any{"script": script, ScriptSourceSHA256Key: ScriptSourceHash(script)}, ""},
		{"non-string script", map[string]any{"script": 1, ScriptSourceKey: "scripts/a.star", ScriptSourceSHA256Key: ScriptSourceHash(script)}, ""},
		{"empty section", map[string]any{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ScriptSourceMatches(tt.section))
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResolveRecordedScriptSource(t *testing.T) {
	base := t.TempDir()
	cfg := &schema.AtmosConfiguration{BasePath: base, BasePathAbsolute: base}
	absolute := filepath.Join(t.TempDir(), "abs.star")

	assert.Equal(t, filepath.Join(base, "scripts", "hook.star"), ResolveRecordedScriptSource(cfg, "scripts/hook.star"))
	assert.Equal(t, absolute, ResolveRecordedScriptSource(cfg, absolute))
	assert.Empty(t, ResolveRecordedScriptSource(cfg, ""))
	assert.Equal(t, filepath.Join(base, "scripts", "hook.star"), ResolveRecordedScriptSource(&schema.AtmosConfiguration{BasePath: base}, "scripts/hook.star"))
}
