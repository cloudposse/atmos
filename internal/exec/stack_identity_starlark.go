package exec

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
)

// identitySections lists the component sections that stacks.name_template and stacks.name_pattern read,
// in the order they are searched when reporting which field made the stack identity computed.
var identitySections = []string{"vars", "settings", "metadata", "env"}

// ensureLiteralStackIdentity rejects a stack name that was derived from a !starlark value.
// A !starlark value only has a result after the stack is known, so it cannot decide the
// stack's own identity. Silently using the encoded source as the name would produce stacks,
// workspaces, and files named after the program text. The error names the manifest and, when
// it can be found, the field whose value the name came from.
func ensureLiteralStackIdentity(stackFileName, name string, section map[string]any) error {
	if !containsStarlark(name) && !strings.Contains(name, starlarksource.Tag+"\n") {
		return nil
	}
	builder := errUtils.Build(errUtils.ErrStarlarkStackIdentity).
		WithContext("manifest", stackFileName)
	explanation := fmt.Sprintf("The stack name for manifest `%s` is derived from a !starlark value.", stackFileName)
	if field := starlarkIdentityField(section, name); field != "" {
		builder = builder.WithContext("field", field)
		explanation = fmt.Sprintf("The stack name for manifest `%s` is derived from `%s`, which is a !starlark value.", stackFileName, field)
	}
	return builder.
		WithExplanation(explanation).
		WithHint("Set a literal value for the fields used by stacks.name_template or stacks.name_pattern.").
		Err()
}

// starlarkIdentityField returns the dotted path (for example "vars.stage") of the !starlark
// value that the stack name was built from, or "" when no field can be tied to the name.
// Only a value whose encoded source appears in the name qualifies, so an unrelated
// !starlark field in the same section is never blamed.
func starlarkIdentityField(section map[string]any, name string) string {
	for _, sectionName := range identitySections {
		values, ok := section[sectionName].(map[string]any)
		if !ok {
			continue
		}
		if path := findStarlarkPath(sectionName, values, name); path != "" {
			return path
		}
	}
	return ""
}

func findStarlarkPath(prefix string, value any, name string) string {
	switch value := value.(type) {
	case string:
		if containsStarlark(value) && strings.Contains(name, value) {
			return prefix
		}
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			if path := findStarlarkPath(prefix+"."+key, value[key], name); path != "" {
				return path
			}
		}
	case []any:
		for i, item := range value {
			if path := findStarlarkPath(prefix+"."+strconv.Itoa(i), item, name); path != "" {
				return path
			}
		}
	}
	return ""
}
