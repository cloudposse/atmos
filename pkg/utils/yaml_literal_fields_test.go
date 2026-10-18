package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a rename of the field these tests rely on must fail the build.
var _ = schema.WorkflowStep{LiteralFields: nil}

// decodeStackSection decodes a stack manifest snippet exactly like stack processing does.
func decodeStackSection(t *testing.T, text string) map[string]any {
	t.Helper()
	out, err := UnmarshalYAMLFromFile[map[string]any](&schema.AtmosConfiguration{}, text, "stack.yaml")
	require.NoError(t, err)
	return out
}

// pick walks a decoded document by string keys and list indexes.
func pick(t *testing.T, doc any, path ...any) any {
	t.Helper()
	current := doc
	for _, part := range path {
		switch key := part.(type) {
		case string:
			m, ok := current.(map[string]any)
			require.True(t, ok, "expected a mapping before %q, got %T", key, current)
			current = m[key]
		case int:
			list, ok := current.([]any)
			require.True(t, ok, "expected a list before index %d, got %T", key, current)
			require.Less(t, key, len(list))
			current = list[key]
		}
	}
	return current
}

func TestMarkLiteralStepFields_HookPayloads(t *testing.T) {
	t.Run("a step hook with payload", func(t *testing.T) {
		doc := decodeStackSection(t, `
hooks:
  notify:
    kind: step
    with:
      type: script
      interpreter: starlark
      script: !literal |
        print("{{ x }}")
`)
		with := pick(t, doc, "hooks", "notify", "with")
		assert.Equal(t, []any{"script"}, pick(t, with, schema.LiteralFieldsKey))
		assert.Equal(t, "print(\"{{ x }}\")\n", pick(t, with, "script"))
	})

	t.Run("a steps hook marks each listed step on its own", func(t *testing.T) {
		doc := decodeStackSection(t, `
components:
  terraform:
    vpc:
      hooks:
        multi:
          kind: steps
          with:
            - type: script
              interpreter: starlark
              script: !literal "print('{{ a }}')"
            - type: shell
              command: echo "{{ b }}"
            - type: shell
              command: !literal echo "{{ c }}"
              env:
                GREETING: !literal "{{ g }}"
                PLAIN: "{{ p }}"
`)
		with := pick(t, doc, "components", "terraform", "vpc", "hooks", "multi", "with")
		assert.Equal(t, []any{"script"}, pick(t, with, 0, schema.LiteralFieldsKey))
		assert.NotContains(t, pick(t, with, 1), schema.LiteralFieldsKey)
		assert.Equal(t, []any{"command", "env.GREETING"}, pick(t, with, 2, schema.LiteralFieldsKey))
		assert.Equal(t, "{{ g }}", pick(t, with, 2, "env", "GREETING"))
	})

	t.Run("nested group steps", func(t *testing.T) {
		doc := decodeStackSection(t, `
hooks:
  group:
    kind: step
    with:
      type: parallel
      steps:
        - type: script
          interpreter: starlark
          script: !literal "print('{{ child }}')"
        - type: script
          interpreter: starlark
          script: "print('{{ other }}')"
`)
		with := pick(t, doc, "hooks", "group", "with")
		assert.NotContains(t, with, schema.LiteralFieldsKey)
		assert.Equal(t, []any{"script"}, pick(t, with, "steps", 0, schema.LiteralFieldsKey))
		assert.NotContains(t, pick(t, with, "steps", 1), schema.LiteralFieldsKey)
	})
}

func TestMarkLiteralStepFields_WorkflowSteps(t *testing.T) {
	doc := decodeStackSection(t, `
workflows:
  demo:
    steps:
      - name: a
        type: script
        script: !literal "print('{{ a }}')"
      - name: g
        type: parallel
        steps:
          - name: child
            command: !literal echo "{{ c }}"
`)
	assert.Equal(t, []any{"script"}, pick(t, doc, "workflows", "demo", "steps", 0, schema.LiteralFieldsKey))
	assert.Equal(t, []any{"command"}, pick(t, doc, "workflows", "demo", "steps", 1, "steps", 0, schema.LiteralFieldsKey))
}

func TestMarkLiteralStepFields_NonStepDataIsUntouched(t *testing.T) {
	doc := decodeStackSection(t, `
vars:
  steps:
    - command: !literal "{{ from vars }}"
  template: !literal "{{ keep }}"
settings:
  steps:
    - script: !literal "{{ from settings }}"
env:
  GREETING: !literal "{{ g }}"
components:
  terraform:
    vpc:
      vars:
        steps:
          - command: !literal "{{ component var }}"
      settings:
        note: !literal "{{ n }}"
      env:
        X: !literal "{{ x }}"
hooks:
  store:
    kind: store
    outputs:
      value: !literal "{{ out }}"
`)
	// The tags are cleared and the values kept exactly, with no extra keys anywhere.
	assert.Equal(t, map[string]any{"command": "{{ from vars }}"}, pick(t, doc, "vars", "steps", 0))
	assert.Equal(t, "{{ keep }}", pick(t, doc, "vars", "template"))
	assert.Equal(t, map[string]any{"script": "{{ from settings }}"}, pick(t, doc, "settings", "steps", 0))
	assert.Equal(t, map[string]any{"GREETING": "{{ g }}"}, pick(t, doc, "env"))
	assert.Equal(t, map[string]any{"command": "{{ component var }}"}, pick(t, doc, "components", "terraform", "vpc", "vars", "steps", 0))
	assert.Equal(t, map[string]any{"note": "{{ n }}"}, pick(t, doc, "components", "terraform", "vpc", "settings"))
	assert.Equal(t, map[string]any{"X": "{{ x }}"}, pick(t, doc, "components", "terraform", "vpc", "env"))
	assert.Equal(t, map[string]any{"value": "{{ out }}"}, pick(t, doc, "hooks", "store", "outputs"))
	assert.NotContains(t, pick(t, doc, "hooks", "store"), schema.LiteralFieldsKey)
}

func TestMarkLiteralStepFields_ReplacesUserWrittenKeyAndIsIdempotent(t *testing.T) {
	text := `
hooks:
  h:
    kind: step
    with:
      type: script
      literal_fields: [command, working_directory]
      script: !literal "print('{{ a }}')"
`
	doc := decodeStackSection(t, text)
	assert.Equal(t, []any{"script"}, pick(t, doc, "hooks", "h", "with", schema.LiteralFieldsKey))

	t.Run("processing a tree twice keeps the loader marker", func(t *testing.T) {
		var root yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte(text), &root))
		require.NoError(t, processCustomTags(&schema.AtmosConfiguration{}, &root, "stack.yaml"))
		require.NoError(t, processCustomTags(&schema.AtmosConfiguration{}, &root, "stack.yaml"))
		var out map[string]any
		require.NoError(t, root.Decode(&out))
		assert.Equal(t, []any{"script"}, pick(t, out, "hooks", "h", "with", schema.LiteralFieldsKey))
	})
}

func TestMarkLiteralStepFields_NilAndScalarRoots(t *testing.T) {
	assert.NotPanics(t, func() { markLiteralStepFields(nil) })
	assert.NotPanics(t, func() { markLiteralStepFields(&yaml.Node{Kind: yaml.ScalarNode, Value: "x"}) })
}

func TestMarkLiteralStepFields_ContainerRunSteps(t *testing.T) {
	t.Run("a workflow container run step marks working_directory", func(t *testing.T) {
		doc := decodeStackSection(t, `
workflows:
  demo:
    steps:
      - name: run
        type: container
        action: run
        working_directory: !literal "{{ w }}"
        with:
          image: alpine
          command: echo
      - name: implicit
        type: container
        working_directory: !literal "{{ w }}"
`)
		assert.Equal(t, []any{"working_directory"}, pick(t, doc, "workflows", "demo", "steps", 0, schema.LiteralFieldsKey))
		assert.Equal(t, "{{ w }}", pick(t, doc, "workflows", "demo", "steps", 0, "working_directory"))
		assert.Equal(t, []any{"working_directory"}, pick(t, doc, "workflows", "demo", "steps", 1, schema.LiteralFieldsKey))
	})

	t.Run("a container step hook marks the with payload by the hook type", func(t *testing.T) {
		doc := decodeStackSection(t, `
hooks:
  sandbox:
    kind: step
    type: container
    with:
      action: run
      working_directory: !literal "{{ w }}"
      image: alpine
`)
		assert.Equal(t, []any{"working_directory"}, pick(t, doc, "hooks", "sandbox", "with", schema.LiteralFieldsKey))
	})

	t.Run("only a container run step qualifies", func(t *testing.T) {
		doc := decodeStackSection(t, `
workflows:
  demo:
    steps:
      - name: build
        type: container
        action: build
        working_directory: !literal "{{ w }}"
      - name: sleep
        type: sleep
        working_directory: !literal "{{ w }}"
      - name: untyped
        working_directory: !literal "{{ w }}"
hooks:
  other:
    kind: step
    type: sleep
    with:
      working_directory: !literal "{{ w }}"
`)
		for _, index := range []int{0, 1, 2} {
			assert.NotContains(t, pick(t, doc, "workflows", "demo", "steps", index), schema.LiteralFieldsKey, "step %d", index)
		}
		assert.NotContains(t, pick(t, doc, "hooks", "other", "with"), schema.LiteralFieldsKey)
		assert.Equal(t, "{{ w }}", pick(t, doc, "hooks", "other", "with", "working_directory"), "the value is still kept exactly")
	})
}

// A user-written literal_fields must never survive on a workflow or hook step. Hook rendering
// trusts that list and skips template processing for the fields it names.
func TestMarkLiteralStepFields_StripsUserWrittenKey(t *testing.T) {
	tests := []struct {
		name string
		text string
		// path locates the step mapping that must hold no literal_fields.
		path []any
	}{
		{
			name: "hook step in a tree with no custom tag at all",
			text: "hooks:\n  h:\n    kind: step\n    with:\n      type: shell\n      literal_fields: [command]\n      command: echo \"{{ a }}\"\n",
			path: []any{"hooks", "h", "with"},
		},
		{
			name: "workflow step in a tree with no custom tag at all",
			text: "workflows:\n  w:\n    steps:\n      - name: s\n        type: shell\n        literal_fields: [command]\n        command: echo \"{{ a }}\"\n",
			path: []any{"workflows", "w", "steps", 0},
		},
		{
			name: "listed hook step in a tree with no custom tag at all",
			text: "hooks:\n  h:\n    kind: steps\n    with:\n      - type: shell\n        literal_fields: [command]\n        command: echo\n",
			path: []any{"hooks", "h", "with", 0},
		},
		{
			name: "step that is not a command or script step",
			text: "workflows:\n  w:\n    steps:\n      - name: s\n        type: sleep\n        literal_fields: [timeout]\n        timeout: 1s\n",
			path: []any{"workflows", "w", "steps", 0},
		},
		{
			name: "hook step with a custom tag elsewhere but no !literal field of its own",
			text: "settings:\n  note: !literal \"{{ keep }}\"\nhooks:\n  h:\n    kind: step\n    with:\n      type: shell\n      literal_fields: [command]\n      command: echo \"{{ a }}\"\n",
			path: []any{"hooks", "h", "with"},
		},
		{
			name: "workflow step with a custom tag elsewhere but no !literal field of its own",
			text: "settings:\n  note: !literal \"{{ keep }}\"\nworkflows:\n  w:\n    steps:\n      - name: s\n        type: shell\n        literal_fields: [command]\n        command: echo \"{{ a }}\"\n",
			path: []any{"workflows", "w", "steps", 0},
		},
		{
			name: "nested workflow step",
			text: "workflows:\n  w:\n    steps:\n      - name: g\n        type: parallel\n        steps:\n          - name: c\n            command: echo\n            literal_fields: [command]\n",
			path: []any{"workflows", "w", "steps", 0, "steps", 0},
		},
		{
			name: "hook step nested under a component",
			text: "components:\n  terraform:\n    vpc:\n      hooks:\n        h:\n          kind: step\n          with:\n            type: shell\n            literal_fields: [command]\n            command: echo\n",
			path: []any{"components", "terraform", "vpc", "hooks", "h", "with"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := decodeStackSection(t, tt.text)
			step := pick(t, doc, tt.path...)
			require.IsType(t, map[string]any{}, step)
			assert.NotContains(t, step, schema.LiteralFieldsKey)
		})
	}
}

func TestMarkLiteralStepFields_LeavesDataSectionsUntouched(t *testing.T) {
	text := `
vars:
  literal_fields: [a, b]
  nested:
    literal_fields: [c]
settings:
  literal_fields: [d]
env:
  literal_fields: e
components:
  terraform:
    vpc:
      vars:
        literal_fields: [f]
`
	t.Run("a tree with no custom tag", func(t *testing.T) {
		doc := decodeStackSection(t, text)
		assert.Equal(t, []any{"a", "b"}, pick(t, doc, "vars", "literal_fields"))
		assert.Equal(t, []any{"c"}, pick(t, doc, "vars", "nested", "literal_fields"))
		assert.Equal(t, []any{"d"}, pick(t, doc, "settings", "literal_fields"))
		assert.Equal(t, "e", pick(t, doc, "env", "literal_fields"))
		assert.Equal(t, []any{"f"}, pick(t, doc, "components", "terraform", "vpc", "vars", "literal_fields"))
	})

	t.Run("a tree with a custom tag and a stripped workflow step", func(t *testing.T) {
		doc := decodeStackSection(t, text+`
workflows:
  w:
    steps:
      - name: s
        command: !literal "{{ x }}"
        literal_fields: [script]
`)
		assert.Equal(t, []any{"a", "b"}, pick(t, doc, "vars", "literal_fields"))
		assert.Equal(t, []any{"c"}, pick(t, doc, "vars", "nested", "literal_fields"))
		assert.Equal(t, []any{"d"}, pick(t, doc, "settings", "literal_fields"))
		assert.Equal(t, []any{"command"}, pick(t, doc, "workflows", "w", "steps", 0, schema.LiteralFieldsKey), "the loader list replaces the user's")
	})
}

// Processing the same node tree again must keep the marker the loader wrote on the first pass, even
// though the !literal tags that produced it are cleared by then, while still dropping a marker that
// came from YAML text.
func TestMarkLiteralStepFields_ReprocessingKeepsLoaderMarker(t *testing.T) {
	process := func(t *testing.T, root *yaml.Node, times int) map[string]any {
		t.Helper()
		for range times {
			require.NoError(t, processCustomTags(&schema.AtmosConfiguration{}, root, "stack.yaml"))
		}
		var out map[string]any
		require.NoError(t, root.Decode(&out))
		return out
	}

	tests := []struct {
		name string
		text string
		path []any
		want any
	}{
		{
			name: "marker on a hook step",
			text: "hooks:\n  h:\n    kind: step\n    with:\n      type: script\n      script: !literal \"{{ a }}\"\n",
			path: []any{"hooks", "h", "with", schema.LiteralFieldsKey},
			want: []any{"script"},
		},
		{
			name: "marker on a workflow step",
			text: "workflows:\n  w:\n    steps:\n      - name: s\n        command: !literal \"{{ a }}\"\n",
			path: []any{"workflows", "w", "steps", 0, schema.LiteralFieldsKey},
			want: []any{"command"},
		},
		{
			name: "marker that replaced a user-written list",
			text: "hooks:\n  h:\n    kind: step\n    with:\n      type: script\n      literal_fields: [command]\n      script: !literal \"{{ a }}\"\n",
			path: []any{"hooks", "h", "with", schema.LiteralFieldsKey},
			want: []any{"script"},
		},
		{
			name: "marker on a container run step",
			text: "workflows:\n  w:\n    steps:\n      - name: s\n        type: container\n        working_directory: !literal \"{{ a }}\"\n",
			path: []any{"workflows", "w", "steps", 0, schema.LiteralFieldsKey},
			want: []any{"working_directory"},
		},
	}
	for _, tt := range tests {
		for _, passes := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("%s after %d passes", tt.name, passes), func(t *testing.T) {
				var root yaml.Node
				require.NoError(t, yaml.Unmarshal([]byte(tt.text), &root))
				assert.Equal(t, tt.want, pick(t, process(t, &root, passes), tt.path...))
			})
		}
	}

	t.Run("a marker parsed from text is stripped on every pass", func(t *testing.T) {
		var root yaml.Node
		text := "hooks:\n  h:\n    kind: step\n    with:\n      type: shell\n      literal_fields: [command]\n      command: echo\n"
		require.NoError(t, yaml.Unmarshal([]byte(text), &root))
		out := process(t, &root, 2)
		assert.NotContains(t, pick(t, out, "hooks", "h", "with"), schema.LiteralFieldsKey)
	})

	t.Run("a marker survives a cache style deep copy between passes", func(t *testing.T) {
		var root yaml.Node
		text := "hooks:\n  h:\n    kind: step\n    with:\n      type: script\n      script: !literal \"{{ a }}\"\n"
		require.NoError(t, yaml.Unmarshal([]byte(text), &root))
		process(t, &root, 1)
		clone := deepCopyYAMLNode(&root)
		assert.Equal(t, []any{"script"}, pick(t, process(t, clone, 1), "hooks", "h", "with", schema.LiteralFieldsKey))
	})
}

func TestScanTagsAndLiteralFieldsKey(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantTagged bool
		wantKey    bool
	}{
		{name: "plain tree", text: "a: 1\nb: [x, y]\n"},
		{name: "custom tag", text: "a: !literal x\n", wantTagged: true},
		{name: "standard tag only", text: "a: !!str 1\n"},
		{name: "literal_fields key", text: "a:\n  literal_fields: [x]\n", wantKey: true},
		{name: "literal_fields as a value is not a key", text: "a: literal_fields\n"},
		{name: "literal_fields in a sequence element", text: "a:\n  - literal_fields: [x]\n", wantKey: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var root yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tt.text), &root))
			tagged, hasKey := scanTagsAndLiteralFieldsKey(&root)
			assert.Equal(t, tt.wantTagged, tagged)
			assert.Equal(t, tt.wantKey, hasKey)
			assert.Equal(t, tagged, hasCustomTags(&root), "the scan agrees with hasCustomTags")
		})
	}
	tagged, hasKey := scanTagsAndLiteralFieldsKey(nil)
	assert.False(t, tagged)
	assert.False(t, hasKey)
}

func TestMarkLiteralStepFields_TimeoutIncludeRawAndGroupEnv(t *testing.T) {
	included := filepath.Join(t.TempDir(), "braces.star")
	require.NoError(t, os.WriteFile(included, []byte("print(\"{{ not a template }}\")\n"), 0o600))
	doc := decodeStackSection(t, fmt.Sprintf(`
workflows:
  demo:
    steps:
      - name: slow
        type: script
        timeout: !literal "{{ t }}"
        script: print("x")
      - name: raw
        type: script
        script: !include.raw %s
      - name: rendered
        type: script
        script: !include %s
      - name: fan
        type: parallel
        env:
          KEPT: !literal "{{ k }}"
          RENDERED: "{{ r }}"
        steps:
          - name: child
            command: echo hi
`, included, included))
	assert.Equal(t, []any{"timeout"}, pick(t, doc, "workflows", "demo", "steps", 0, schema.LiteralFieldsKey))
	assert.Equal(t, []any{"script"}, pick(t, doc, "workflows", "demo", "steps", 1, schema.LiteralFieldsKey),
		"an !include.raw script is used as written")
	assert.NotContains(t, pick(t, doc, "workflows", "demo", "steps", 2), schema.LiteralFieldsKey,
		"a plain !include script is still rendered")
	assert.Equal(t, []any{"env.KEPT"}, pick(t, doc, "workflows", "demo", "steps", 3, schema.LiteralFieldsKey))
}
