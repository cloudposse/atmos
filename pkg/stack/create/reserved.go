package create

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// verifyReservedInputs checks that the rendered manifest actually used the stack flags the user passed. A
// recipe that never references .Config.stack_name, .Config.stack_imports or .Config.stack_components would
// otherwise drop them without a word and produce a stack that is not what was asked for.
func verifyReservedInputs(template string, opts *Options, declared []declaredComponent, document map[string]any) error {
	return verifyReservedInputsAcross(template, opts, declared, []map[string]any{document})
}

// verifyReservedInputsAcross allows a directory scaffold to place each input in a different stack document.
func verifyReservedInputsAcross(template string, opts *Options, declared []declaredComponent, documents []map[string]any) error {
	nameFound, imported, components := consumedInputs(opts.Name, declared, documents)
	if !nameFound {
		return unusedInputError(template, "--name", "stack_name", "name", opts.Name)
	}
	for _, raw := range opts.Imports {
		if path := strings.TrimSpace(raw); !imported[path] {
			return unusedInputError(template, "--import", "stack_imports", "import", path)
		}
	}
	for _, component := range declared {
		if !components[component] {
			return unusedInputError(template, "--component", "stack_components", "components", component.kind+":"+component.name)
		}
	}
	return nil
}

func unusedInputError(template, flag, input, section, value string) error {
	return errUtils.Build(fmt.Errorf("%w: template %q does not use %s; %s %q was not written to the %s section of the manifest",
		errUtils.ErrInvalidFlag, template, flag, flag, value, section)).
		WithHint(fmt.Sprintf("Use a template that consumes `.Config.%s`, or remove `%s`", input, flag)).
		Err()
}

// importedPaths collects the paths of a manifest's import list, which may hold plain paths or maps with a path key.
func importedPaths(value any) map[string]bool {
	paths := make(map[string]bool)
	entries, _ := value.([]any)
	for _, entry := range entries {
		switch typed := entry.(type) {
		case string:
			paths[strings.TrimSpace(typed)] = true
		case map[string]any:
			if path, ok := typed["path"].(string); ok {
				paths[strings.TrimSpace(path)] = true
			}
		}
	}
	return paths
}

func hasComponent(document map[string]any, component declaredComponent) bool {
	components, _ := document["components"].(map[string]any)
	kind, _ := components[component.kind].(map[string]any)
	_, found := kind[component.name]
	return found
}

func consumedInputs(name string, declared []declaredComponent, documents []map[string]any) (bool, map[string]bool, map[declaredComponent]bool) {
	nameFound := name == ""
	imported := make(map[string]bool)
	components := make(map[declaredComponent]bool)
	for _, document := range documents {
		if value, _ := document["name"].(string); value == name {
			nameFound = true
		}
		for path := range importedPaths(document["import"]) {
			imported[path] = true
		}
		for _, component := range declared {
			if hasComponent(document, component) {
				components[component] = true
			}
		}
	}
	return nameFound, imported, components
}
