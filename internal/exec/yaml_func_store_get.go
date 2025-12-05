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

type getKeyParams struct {
	storeName    string
	key          string
	query        string
	defaultValue *string
}

// extractPipeParams extracts default value and query from pipe-separated parts.
func extractPipeParams(parts []string, input string) (defaultValue *string, query string, err error) {
	for _, p := range parts[1:] {
		pipeParts := strings.Fields(strings.TrimSpace(p))
		if len(pipeParts) != 2 {
			log.Error(invalidYamlFuncMsg, function, input, "invalid number of parameters after the pipe", len(pipeParts))
			return nil, "", fmt.Errorf("%w: %s", ErrInvalidPipeParams, input)
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
			return nil, "", fmt.Errorf("%w: %s", ErrInvalidPipeIdentifier, input)
		}
	}
	return defaultValue, query, nil
}

func processTagStoreGet(atmosConfig *schema.AtmosConfiguration, input string, currentStack string) any {
	defer perf.Track(atmosConfig, "exec.processTagStoreGet")()

	log.Debug("Executing Atmos YAML function", "function", input)
	log.Debug("Processing !store.get", "input", input, "tag", u.AtmosYamlFuncStoreGet)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStoreGet)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "", "")
		return nil
	}
	log.Debug("After getStringAfterTag", "str", str)

	// Split the input on the pipe symbol to separate the store parameters and default value.
	parts := strings.Split(str, "|")
	storePart := strings.TrimSpace(parts[0])

	// Extract default value and query from pipe parts.
	var defaultValue *string
	var query string
	if len(parts) > 1 {
		defaultValue, query, err = extractPipeParams(parts, input)
		if err != nil {
			return err.Error()
		}
	}

	// Process the main store part.
	storeParts := strings.Fields(storePart)
	partsLength := len(storeParts)
	if partsLength != 2 {
		log.Error(invalidYamlFuncMsg, function, input, "invalid number of parameters", partsLength)
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
	}

	retParams := getKeyParams{
		storeName:    strings.TrimSpace(storeParts[0]),
		key:          strings.TrimSpace(storeParts[1]),
		defaultValue: defaultValue,
		query:        query,
	}

	// Retrieve the store from atmosConfig.
	store := atmosConfig.Stores[retParams.storeName]
	if store == nil {
		er := fmt.Errorf("failed to execute YAML function %s. %w: %s", input, ErrStoreNotFound, retParams.storeName)
		errUtils.CheckErrorPrintAndExit(er, "", "")
		return nil
	}

	// Retrieve the value from the store using the arbitrary key.
	value, err := store.GetKey(retParams.key)
	if err != nil {
		if retParams.defaultValue != nil {
			return *retParams.defaultValue
		}
		er := fmt.Errorf("%w: failed to execute YAML function %s for key %s: %w", ErrGetKeyFailed, input, retParams.key, err)
		errUtils.CheckErrorPrintAndExit(er, "", "")
		return nil
	}

	// Check if the retrieved value is nil and use default if provided.
	// This handles the case where nil was stored (e.g., from rate limit failures).
	if value == nil && retParams.defaultValue != nil {
		return *retParams.defaultValue
	}

	// Execute the YQ expression if provided.
	res := value
	if retParams.query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, value, retParams.query)
		if err != nil {
			errUtils.CheckErrorPrintAndExit(err, "", "")
			return nil
		}
	}

	return res
}
