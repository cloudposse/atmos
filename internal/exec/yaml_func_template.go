package exec

import (
	"encoding/json"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(
	input string,
) any {
	defer perf.Track(nil, "exec.processTagTemplate")()

	log.Debug("Executing", "Atmos YAML function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTemplate)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	var decoded any
	if err = json.Unmarshal([]byte(str), &decoded); err != nil {
		return str
	}

	return decoded
}

// ProcessTemplateTagsOnly processes only !template tags in a data structure, recursively.
// It is used before merging to ensure !template strings are decoded to their actual types.
// This avoids type conflicts during merge (e.g., string vs list).
func ProcessTemplateTagsOnly(input map[string]any) map[string]any {
	defer perf.Track(nil, "exec.ProcessTemplateTagsOnly")()

	if input == nil {
		return nil
	}

	result := make(map[string]any, len(input))

	var recurse func(any) any
	recurse = func(node any) any {
		switch v := node.(type) {
		case string:
			// Only process !template tags, leave other tags as-is.
			if strings.HasPrefix(v, u.AtmosYamlFuncTemplate) {
				return processTagTemplate(v)
			}
			return v

		case map[string]any:
			newMap := make(map[string]any, len(v))
			for k, val := range v {
				newMap[k] = recurse(val)
			}
			return newMap

		case []any:
			newSlice := make([]any, len(v))
			for i, val := range v {
				newSlice[i] = recurse(val)
			}
			return newSlice

		default:
			return v
		}
	}

	for k, v := range input {
		result[k] = recurse(v)
	}

	return result
}
