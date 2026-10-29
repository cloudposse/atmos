package doctor

import (
	"strconv"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
)

// functionContext distinguishes document consumers and their narrow field exceptions.
func functionContext(d *document, path []string) fntag.Context {
	switch d.kind {
	case documentConfig:
		return configFunctionContext(path)
	case documentVendor:
		return fntag.VendorContext
	case documentScaffold:
		return fntag.ScaffoldContext
	case documentAtmosWorkflow:
		return workflowFunctionContext(path)
	case documentStack:
		return stackFunctionContext(path)
	}
	return ""
}

func configFunctionContext(path []string) fntag.Context {
	if len(path) == 4 && path[0] == "ai" && path[1] == "skills" && path[3] == "ref" {
		return fntag.SkillRefContext
	}
	if conditionField(path) || (len(path) >= 3 && path[0] == "import" && path[2] == "when") {
		return fntag.ConfigConditionContext
	}
	return fntag.ConfigContext
}

func workflowFunctionContext(path []string) fntag.Context {
	if conditionField(path) {
		return fntag.ConditionContext
	}
	if workflowInteractiveField(path) {
		return fntag.WorkflowInteractiveContext
	}
	return fntag.WorkflowContext
}

// workflowInteractiveField follows the same nested step lists as the runtime resolver.
func workflowInteractiveField(path []string) bool {
	const interactiveFieldDepth = 5
	if len(path) < interactiveFieldDepth || path[0] != "workflows" {
		return false
	}
	i := 2
	for i+1 < len(path) && path[i] == "steps" {
		if _, err := strconv.Atoi(path[i+1]); err != nil {
			return false
		}
		i += 2
	}
	if i >= len(path) {
		return false
	}
	switch path[i] {
	case "default", "prompt", "placeholder":
		return i+1 == len(path)
	case "options":
		if i+2 != len(path) {
			return false
		}
		_, err := strconv.Atoi(path[i+1])
		return err == nil
	}
	return false
}

// conditionField requires a declared condition-bearing section rather than
// mistaking arbitrary user data with a key named "when" for a condition.
func conditionField(path []string) bool {
	for i, part := range path {
		if part != "when" && part != "continue" {
			continue
		}
		owner := path[:i]
		if stepOwner(owner) || hookOwner(owner) {
			return true
		}
	}
	return false
}

func stepOwner(path []string) bool {
	if len(path) < 4 {
		return false
	}
	i := stepOwnerStart(path)
	return i > 0 && consumeIndexedPath(path, i, "steps") == len(path)
}

func stepOwnerStart(path []string) int {
	switch path[0] {
	case keyWorkflows:
		return 2
	case "commands":
		return consumeIndexedPath(path, 0, "commands")
	default:
		for j := range path {
			if path[j] == "steps" && hookOwner(path[:j]) {
				return j
			}
		}
	}
	return -1
}

func consumeIndexedPath(path []string, i int, section string) int {
	for i+1 < len(path) && path[i] == section {
		if _, err := strconv.Atoi(path[i+1]); err != nil {
			return -1
		}
		i += 2
	}
	return i
}

func hookOwner(path []string) bool {
	if len(path) < 2 || path[len(path)-2] != "hooks" {
		return false
	}
	prefix := path[:len(path)-2]
	return len(prefix) == 0 || (len(prefix) == 1 && oneOfComponentType(prefix[0])) ||
		(len(prefix) == 3 && prefix[0] == "components" && oneOfComponentType(prefix[1]))
}
