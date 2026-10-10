package step

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
)

func TestEveryBuiltInStepDeclaresKnownFields(t *testing.T) {
	require.NotEmpty(t, builtInStepTypes, "the registry inventory is captured by TestMain")
	for _, name := range builtInStepTypes {
		handler, ok := Get(name)
		require.True(t, ok, name)
		declared, ok := handler.(knownFieldsHandler)
		assert.True(t, ok, "step type %q must implement KnownFields so a direct call rejects foreign fields", name)
		if ok {
			assert.NotNil(t, declared.KnownFields(), name)
		}
	}
}

func TestKnownFieldsCannotBeMutatedThroughTheReturnedSlice(t *testing.T) {
	handler, _ := Get("shell")
	first := handler.(knownFieldsHandler).KnownFields()
	require.NotEmpty(t, first)
	original := first[0]
	first[0] = "mutated"
	assert.Equal(t, original, handler.(knownFieldsHandler).KnownFields()[0])
}

func TestStepYAMLFieldsPanicsOnUnknownGoField(t *testing.T) {
	assert.Equal(t, []string{"working_directory"}, stepYAMLFields("WorkingDirectory"))
	assert.Panics(t, func() { stepYAMLFields("NoSuchField") })
	assert.Panics(t, func() { stepYAMLFields("LoadError") }, "a field without a YAML key cannot be named")
}

// A field that belongs to another step type used to be accepted and ignored.
func TestDirectStepCallRejectsFieldsOfOtherStepTypes(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)
	for _, tc := range []struct {
		name      string
		stepType  string
		fields    map[string]any
		wantField string
		wantValid string
	}{
		{name: "url on shell", stepType: "shell", fields: map[string]any{"command": "echo hi", "url": "x"}, wantField: `"url"`, wantValid: "command"},
		{name: "command on join", stepType: "join", fields: map[string]any{"options": []string{"a"}, "command": "x"}, wantField: `"command"`, wantValid: "separator"},
		{name: "prompt on shell", stepType: "shell", fields: map[string]any{"command": "echo hi", "prompt": "x"}, wantField: `"prompt"`, wantValid: "command"},
		{name: "duration on sleep", stepType: "sleep", fields: map[string]any{"duration": "1s"}, wantField: `"duration"`, wantValid: "timeout"},
		{name: "options on input", stepType: "input", fields: map[string]any{"prompt": "x", "options": []string{"a"}}, wantField: `"options"`, wantValid: "placeholder"},
		{name: "image on atmos", stepType: "atmos", fields: map[string]any{"command": "version", "image": "x"}, wantField: `"image"`, wantValid: "stack"},
		{name: "method on exec", stepType: "exec", fields: map[string]any{"command": "x", "method": "GET"}, wantField: `"method"`, wantValid: "command"},
		{name: "separator on log", stepType: "log", fields: map[string]any{"content": "x", "separator": ","}, wantField: `"separator"`, wantValid: "level"},
		{name: "alias resolves to the canonical handler", stepType: "webhook", fields: map[string]any{"url": "http://127.0.0.1:1", "command": "x"}, wantField: `"command"`, wantValid: "headers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := library.Validate(&automation.StepCall{Type: tc.stepType, Configuration: tc.fields})
			require.ErrorIs(t, err, errUtils.ErrAutomation)
			assert.Contains(t, err.Error(), tc.wantField)
			assert.Contains(t, err.Error(), `step type "`+tc.stepType+`"`, "the message names the step type")
			assert.Contains(t, err.Error(), tc.wantValid, "the message lists the valid fields")
		})
	}
}

// Fields a step type reads, and the common fields, stay accepted.
func TestDirectStepCallAcceptsDeclaredFields(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)
	for _, tc := range []struct {
		stepType string
		fields   map[string]any
	}{
		{"shell", map[string]any{"command": "echo hi", "env": map[string]string{"A": "b"}, "timeout": "5s", "working_directory": ".", "retry": map[string]any{"max_attempts": 2}, "output": "none"}},
		{"join", map[string]any{"options": []string{"a"}, "separator": ",", "outputs": map[string]string{"x": "{{ .value }}"}}},
		{"http", map[string]any{"url": "http://127.0.0.1:1", "method": "GET", "headers": map[string]string{"A": "b"}}},
		{"sleep", map[string]any{"timeout": "1ms"}},
		{"atmos", map[string]any{"command": "version", "stack": "dev"}},
		{"container", map[string]any{"action": "run", "with": map[string]any{"image": "alpine:3.20", "command": "echo hi"}}},
		{"script", map[string]any{"interpreter": "starlark", "script": "print(1)"}},
	} {
		t.Run(tc.stepType, func(t *testing.T) {
			require.NoError(t, library.Validate(&automation.StepCall{Type: tc.stepType, Configuration: tc.fields}))
		})
	}
}

// A scheduler policy keeps its explanatory message instead of reading as a misspelled field.
func TestDirectStepCallKeepsSchedulerPolicyMessage(t *testing.T) {
	library := NewAutomationLibrary(nil, nil)
	err := library.Validate(&automation.StepCall{Type: "shell", Configuration: map[string]any{"command": "echo hi", "needs": []string{"a"}}})
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	assert.Contains(t, err.Error(), "needs is not supported in a direct step call")
}

// Every step field the step reference documents for a type must be accepted by that type's direct
// call, so a field list that falls behind the documentation fails here instead of in a user's
// script. Only keys that are step fields are checked: the reference also uses definition lists for
// the parameters nested under `with`, `fail`, and `output`.
func TestDocumentedStepFieldsAreAccepted(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	typeRoot := filepath.Join(root, "website", "docs", "steps", "type")
	definition := regexp.MustCompile(`<dt>([^<]*)</dt>`)
	identifier := regexp.MustCompile("`([a-z][a-z0-9_]*)`")
	stepFields := automationStepFields()
	// A direct call deliberately rejects these: they need the enclosing workflow, command, or hook.
	enclosing := map[string]bool{"container": true, "background": true, "needs": true}
	// These pages list the parameters nested under `with` or `fail` as definitions too, and several
	// of those share a name with a step field (`command`, `shell`, `source`, `mode`, `key`).
	nested := map[string]bool{"container": true, "parallel": true, "matrix": true, "store": true}
	// The wait page documents wait-all and wait together; the other pages map one to one.
	pages := map[string]string{"wait-all": "wait"}
	checked := 0
	for _, name := range builtInStepTypes {
		if nested[name] {
			continue
		}
		page := name
		if mapped, ok := pages[name]; ok {
			page = mapped
		}
		data, err := os.ReadFile(filepath.Join(typeRoot, page+".mdx"))
		require.NoError(t, err, name)
		fields, scoped := automationFieldsFor(name)
		require.True(t, scoped, name)
		for _, match := range definition.FindAllStringSubmatch(string(data), -1) {
			for _, token := range identifier.FindAllStringSubmatch(match[1], -1) {
				field := token[1]
				if !stepFields[field] || enclosing[field] || (name == "wait-all" && field == "for") {
					continue
				}
				checked++
				assert.True(t, fields[field], "step type %q documents field %q that its direct call rejects", name, field)
			}
		}
	}
	require.Positive(t, checked, "the documentation scan found no step fields; the page layout may have changed")
}
