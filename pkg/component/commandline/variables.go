package commandline

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ParseVariables parses command-line arguments and extracts all -var arguments,
// returning them as a map of variables with proper type conversion.
// This function processes JSON values and returns them as parsed objects.
// Example: ["-var", "name=test", "-var", "tags={\"env\":\"prod\",\"team\":\"devops\"}"]
// Returns: map[string]any{"name": "test", "tags": map[string]any{"env": "prod", "team": "devops"}}.
func ParseVariables(args []string) (map[string]any, error) {
	defer perf.Track(nil, "commandline.ParseVariables")()

	variables := make(map[string]any)
	for i := 0; i < len(args); i++ {
		if args[i] != "-var" || i+1 >= len(args) {
			continue
		}
		i++
		name, raw, found := strings.Cut(args[i], flagValueSeparator)
		if !found {
			continue
		}
		value, err := parseVariableValue(raw)
		if err != nil {
			return nil, err
		}
		variables[name] = value
	}
	return variables, nil
}

func parseVariableValue(raw string) (any, error) {
	if filetype.IsJSON(raw) {
		return u.ConvertFromJSON(raw)
	}
	return strings.TrimSpace(raw), nil
}
