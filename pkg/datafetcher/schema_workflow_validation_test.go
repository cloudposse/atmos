package datafetcher

import (
	"testing"
)

// These tests reproduce the exact false-positive validation errors reported in issue #2708
// against the published (website) manifest schema, plus a representative sample of the many
// step types whose fields were previously missing. They fail if the schema rejects a valid,
// working workflow — the user-visible symptom of the schema drift.

// TestManifestSchema_Issue2708_WorkflowDependenciesTools reproduces the first snippet: a
// per-workflow tool declaration (`dependencies.tools`) plus a plain command step.
func TestManifestSchema_Issue2708_WorkflowDependenciesTools(t *testing.T) {
	website := loadWebsiteSchemaBytes(t)
	manifest := map[string]any{
		"workflows": map[string]any{
			"build-manifests-ci": map[string]any{
				"description": "Build manifests",
				"dependencies": map[string]any{
					"tools": map[string]any{
						"kustomize": "5.0.3",
					},
				},
				"steps": []any{
					map[string]any{"command": "kustomize build ."},
				},
			},
		},
	}
	assertSchemaValid(t, website, manifest)
}

// TestManifestSchema_Issue2708_InteractiveChooseAndInputSteps reproduces the second snippet:
// `type: choose` with `options` + `default`, and `type: input` with `default`.
func TestManifestSchema_Issue2708_InteractiveChooseAndInputSteps(t *testing.T) {
	website := loadWebsiteSchemaBytes(t)
	manifest := map[string]any{
		"workflows": map[string]any{
			"build-manifests": map[string]any{
				"description": "Interactively pick a target",
				"steps": []any{
					map[string]any{
						"name":    "account",
						"type":    "choose",
						"prompt":  "Account",
						"options": []any{"dev", "prod"},
						"default": "dev",
					},
					map[string]any{
						"name":    "tag",
						"type":    "input",
						"prompt":  "Release tag",
						"default": "latest",
					},
					map[string]any{"command": "kustomize build ."},
				},
			},
		},
	}
	assertSchemaValid(t, website, manifest)
}

// TestManifestSchema_WorkflowStepTypeFields validates a representative field from each of the
// major step-type families that shipped in 1.222.0, guarding the whole class of drift the issue
// describes rather than only the three fields it names.
func TestManifestSchema_WorkflowStepTypeFields(t *testing.T) {
	website := loadWebsiteSchemaBytes(t)
	cases := map[string]map[string]any{
		"script":                     {"type": "script", "script": "echo hi", "interpreter": "bash"},
		"interactive tty":            {"command": "vim", "interactive": true, "tty": true},
		"input placeholder/password": {"type": "input", "prompt": "PW", "placeholder": "secret", "password": true},
		"choose multiple/limit":      {"type": "choose", "options": []any{"a", "b"}, "multiple": true, "limit": 2},
		"file picker":                {"type": "file", "path": ".", "extensions": []any{".yaml"}},
		"workdir source":             {"type": "workdir", "path": "build", "source": "git::https://example.com/repo.git", "reset": true},
		"table":                      {"type": "table", "columns": []any{"a"}, "data": []any{map[string]any{"a": "1"}}},
		"style":                      {"type": "style", "content": "hi", "foreground": "212", "border": "rounded", "bold": true, "padding": "1 2", "align": "center"},
		"log fields":                 {"type": "log", "level": "info", "fields": map[string]any{"k": "v"}},
		"say voice":                  {"type": "say", "text": "done", "voice": []any{"Alex"}, "print": "always"},
		"env vars":                   {"type": "env", "vars": map[string]any{"FOO": "bar"}},
		"exit code":                  {"type": "exit", "code": 1},
		"http request":               {"type": "http", "url": "https://example.com", "method": "POST", "headers": map[string]any{"X": "y"}, "body": "{}", "expect": map[string]any{"status": []any{200}}},
		"cast session":               {"type": "cast", "mode": "session", "shell": "bash", "write_rate": "20ms", "key_interval": "5ms"},
		"container run":              {"type": "container", "action": "run", "provider": "docker", "with": map[string]any{"image": "alpine", "command": "echo hi"}},
		"container async background": {"type": "container", "action": "run", "background": true, "with": map[string]any{"image": "alpine"}},
		"style string background":    {"type": "style", "content": "hi", "background": "236"},
		"emulator":                   {"type": "emulator", "component": "aws", "action": "up", "ephemeral": true},
		"junit files":                {"type": "junit", "files": []any{"reports/*.xml"}},
		"require tools/dirs":         {"type": "require", "tools": []any{"kubectl"}, "dirs": []any{"."}, "hint": "install it"},
		"outputs":                    {"command": "echo hi", "outputs": map[string]any{"result": "{{ .output }}"}},
		"wait action for":            {"type": "wait", "for": []any{"build"}},
		"viewport/show":              {"command": "echo hi", "viewport": map[string]any{"height": 20}, "show": map[string]any{"command": true}},
	}
	for name, step := range cases {
		t.Run(name, func(t *testing.T) {
			assertSchemaValid(t, website, workflowManifestWithStep(step))
		})
	}
}

// TestManifestSchema_WorkflowManifestLevelFields validates the workflow-definition (not step)
// level fields that were missing (`working_directory`, `env`, `container`, `output`, `viewport`,
// `show`) alongside `dependencies`.
func TestManifestSchema_WorkflowManifestLevelFields(t *testing.T) {
	website := loadWebsiteSchemaBytes(t)
	manifest := map[string]any{
		"workflows": map[string]any{
			"full": map[string]any{
				"description":       "Every workflow-level field.",
				"working_directory": "build",
				"stack":             "dev",
				"env":               map[string]any{"FOO": "bar"},
				"container":         map[string]any{"image": "alpine"},
				"output":            "grouped",
				"viewport":          map[string]any{"height": 30, "width": 120},
				"show":              map[string]any{"header": true, "labels": false},
				"dependencies": map[string]any{
					"tools": map[string]any{"terraform": "1.9.0"},
				},
				"steps": []any{
					map[string]any{"command": "echo ok"},
				},
			},
		},
	}
	assertSchemaValid(t, website, manifest)

	// container: false (run on host) must also be accepted.
	hostManifest := map[string]any{
		"workflows": map[string]any{
			"host": map[string]any{
				"container": false,
				"steps":     []any{map[string]any{"command": "echo ok"}},
			},
		},
	}
	assertSchemaValid(t, website, hostManifest)
}

// TestManifestSchema_WorkflowStepAllowsUnknownFields documents that the workflow_step definition
// intentionally uses additionalProperties:true. Workflow steps are a lenient, polymorphic union of
// many step types, and Atmos ignores unknown keys at runtime, so a strict schema would draw
// false-positive errors on valid, evolving workflows (issue #2708). Known fields still exist for
// autocomplete and type checks (see TestManifestSchema_WorkflowStepTypedFieldsStillValidated).
func TestManifestSchema_WorkflowStepAllowsUnknownFields(t *testing.T) {
	website := loadWebsiteSchemaBytes(t)
	assertSchemaValid(t, website, workflowManifestWithStep(map[string]any{
		"command":                  "echo ok",
		"some_future_step_field":   "value",
		"another_unmodeled_option": true,
	}))
}

// TestManifestSchema_WorkflowStepTypedFieldsStillValidated confirms that even with
// additionalProperties:true, the typed schemas on known fields are still enforced, so genuine type
// errors (e.g. a scalar where a list is expected) are caught.
func TestManifestSchema_WorkflowStepTypedFieldsStillValidated(t *testing.T) {
	website := loadWebsiteSchemaBytes(t)
	// options must be an array of strings.
	assertSchemaInvalid(t, website, workflowManifestWithStep(map[string]any{
		"type":    "choose",
		"options": "not-a-list",
	}))
	// code must be an integer.
	assertSchemaInvalid(t, website, workflowManifestWithStep(map[string]any{
		"type": "exit",
		"code": "not-a-number",
	}))
}
