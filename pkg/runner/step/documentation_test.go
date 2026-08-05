package step

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// builtInStepTypes captures production registrations before tests can register
// temporary handlers. The documentation inventory must track the canonical
// public registry, not a handler created by an individual unit test.
var builtInStepTypes = func() []string {
	types := make([]string, 0, len(registry.handlers))
	for name := range registry.handlers {
		types = append(types, name)
	}
	sort.Strings(types)
	return types
}()

func TestStepDocumentationCoversRegisteredTypes(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	stepsRoot := filepath.Join(root, "website", "docs", "workflows", "workflows", "workflow", "steps")
	typeRoot := filepath.Join(stepsRoot, "type")
	index := readDocumentationFile(t, filepath.Join(stepsRoot, "type.mdx"))
	sidebar := readDocumentationFile(t, filepath.Join(root, "website", "sidebars.js"))

	// wait-all is documented with wait because both types operate on the same
	// background-service model. Every other canonical registry type owns a page.
	documents := map[string]string{
		"alert": "alert", "archive": "archive", "atmos": "atmos", "cancel": "cancel",
		"cast": "cast", "choose": "choose", "clear": "clear", "confirm": "confirm",
		"container": "container", "emulator": "emulator", "env": "env", "exec": "exec",
		"exit": "exit", "file": "file", "filter": "filter", "format": "format",
		"hint": "hint", "http": "http", "input": "input", "join": "join", "junit": "junit",
		"linebreak": "linebreak", "log": "log", "markdown": "markdown", "matrix": "matrix",
		"pager": "pager", "parallel": "parallel", "require": "require", "say": "say",
		"script": "script", "shell": "shell", "sleep": "sleep", "spin": "spin",
		"stage": "stage", "style": "style", "table": "table", "title": "title",
		"toast": "toast", "wait": "wait", "wait-all": "wait", "workdir": "workdir", "write": "write",
	}

	for _, stepType := range builtInStepTypes {
		doc, ok := documents[stepType]
		if !ok {
			t.Fatalf("registered step type %q has no documentation inventory entry", stepType)
		}
		if _, err := os.Stat(filepath.Join(typeRoot, doc+".mdx")); err != nil {
			t.Fatalf("registered step type %q is missing %s.mdx: %v", stepType, doc, err)
		}
		if !strings.Contains(index, "/workflows/steps/type/"+doc) {
			t.Errorf("registered step type %q is missing from the public type index", stepType)
		}
		if !strings.Contains(sidebar, "steps/type/"+doc) {
			t.Errorf("registered step type %q is missing from the documentation sidebar", stepType)
		}
	}

	for alias, canonical := range map[string]string{"assert": "require", "webhook": "http"} {
		content := readDocumentationFile(t, filepath.Join(typeRoot, canonical+".mdx"))
		if !strings.Contains(content, "`"+alias+"`") {
			t.Errorf("alias %q is not documented on %s.mdx", alias, canonical)
		}
	}
}

func readDocumentationFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read documentation file %s: %v", path, err)
	}
	return string(data)
}
