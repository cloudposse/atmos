package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type params struct {
	storeName    string
	stack        string
	component    string
	key          string
	query        string
	defaultValue *string
}

func processTagStore(atmosConfig *schema.AtmosConfiguration, input string, currentStack string) (any, error) {
	defer perf.Track(atmosConfig, "exec.processTagStore")()

	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStore)
	if err != nil {
		return nil, err
	}

	// Split the input on the pipe symbol to separate the store parameters and default value.
	parts := strings.Split(str, "|")
	storePart := strings.TrimSpace(parts[0])

	// Default value and query.
	var defaultValue *string
	var query string
	if len(parts) > 1 {
		// Expecting the format: default <value> or query <yq-expression>.
		for _, p := range parts[1:] {
			pipeParts := strings.Fields(strings.TrimSpace(p))
			if len(pipeParts) != 2 {
				log.Error(invalidYamlFuncMsg, function, input, "invalid number of parameters after the pipe", len(pipeParts))
				return nil, fmt.Errorf("%w: %s: invalid number of parameters after the pipe: %d", errUtils.ErrYamlFuncInvalidArguments, input, len(pipeParts))
			}
			v1 := strings.Trim(pipeParts[0], `"'`) // Remove surrounding quotes if present.
			v2 := strings.Trim(pipeParts[1], `"'`)
			switch v1 {
			case "default":
				defaultValue = &v2
			case "query":
				query = v2
			default:
				log.Error(invalidYamlFuncMsg, function, input, "invalid identifier after the pipe", v1)
				return nil, fmt.Errorf("%w: %s: invalid identifier after the pipe: %s", errUtils.ErrYamlFuncInvalidArguments, input, v1)
			}
		}
	}

	// Process the main store part.
	storeParts := strings.Fields(storePart)
	partsLength := len(storeParts)
	if partsLength != 3 && partsLength != 4 {
		log.Error(invalidYamlFuncMsg, function, input, "invalid number of parameters", partsLength)
		return nil, fmt.Errorf("%w: %s: invalid number of parameters: %d", errUtils.ErrYamlFuncInvalidArguments, input, partsLength)
	}

	retParams := params{
		storeName:    strings.TrimSpace(storeParts[0]),
		defaultValue: defaultValue,
		query:        query,
	}

	if partsLength == 4 {
		retParams.stack = strings.TrimSpace(storeParts[1])
		retParams.component = strings.TrimSpace(storeParts[2])
		retParams.key = strings.TrimSpace(storeParts[3])
	} else if partsLength == 3 {
		retParams.stack = currentStack
		retParams.component = strings.TrimSpace(storeParts[1])
		retParams.key = strings.TrimSpace(storeParts[2])
	}

	// Retrieve the store from atmosConfig.
	store := atmosConfig.Stores[retParams.storeName]

	if store == nil {
		return nil, fmt.Errorf("%w: store %s not found in function %s", ErrStoreNotFound, retParams.storeName, input)
	}

	// Retrieve the value from the store.
	value, err := store.Get(retParams.stack, retParams.component, retParams.key)
	if err != nil {
		if retParams.defaultValue != nil {
			return *retParams.defaultValue, nil
		}
		return nil, fmt.Errorf("error executing YAML function %s: failed to get key %s: %w", input, retParams.key, err)
	}

	// Execute the YQ expression if provided.
	res := value

	if retParams.query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, value, retParams.query)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}
