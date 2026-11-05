package doctor

import (
	"slices"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
)

func stackFunctionContext(path []string) fntag.Context {
	if conditionField(path) {
		return fntag.StackConditionContext
	}
	if len(path) == 0 {
		return fntag.StackUnknownContext
	}
	if path[0] == "import" || path[0] == "imports" {
		return fntag.ImportContext
	}
	section, component := stackValuePath(path)
	if len(section) == 0 {
		return fntag.StackUnknownContext
	}
	if section[0] == "overrides" {
		return stackSectionContext(section[1:], component)
	}
	return stackSectionContext(section, component)
}

// stackValuePath removes the enclosing component or component-type defaults.
func stackValuePath(path []string) ([]string, bool) {
	const componentSectionDepth = 3
	if path[0] == "components" {
		if len(path) <= componentSectionDepth {
			return nil, true
		}
		return path[componentSectionDepth:], true
	}
	if oneOfComponentType(path[0]) || path[0] == "cloudformation" {
		return path[1:], true
	}
	return path, false
}

func stackSectionContext(path []string, component bool) fntag.Context {
	if len(path) == 0 {
		return fntag.StackUnknownContext
	}
	switch path[0] {
	case "component", "inheritance", "name":
		return fntag.StackStructuralContext
	case "mocks", "sources":
		return fntag.StackCopiedContext
	case "metadata":
		return stackMetadataContext(path[1:])
	}
	global := []string{"vars", "settings", "env", "auth", "hooks", "retry", "secrets", "generate"}
	if slices.Contains(global, path[0]) {
		return fntag.StackContext
	}
	// These component sections survive the built-in merge and reach value evaluation.
	sections := []string{"backend", "backend_type", "remote_state_backend", "remote_state_backend_type", "command", "dependencies", "locals", "providers", "required_providers", "required_version", "source", "provision", "test", "plugins", "values", "values_files", "chart", "namespace", "manifests", "image", "run", "build", "container"}
	if component && slices.Contains(sections, path[0]) {
		return fntag.StackContext
	}
	return fntag.StackUnknownContext
}

func stackMetadataContext(path []string) fntag.Context {
	if len(path) > 0 && slices.Contains([]string{"component", "inherits", "type", "name"}, path[0]) {
		return fntag.StackStructuralContext
	}
	return fntag.StackContext
}
