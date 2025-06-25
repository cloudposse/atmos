package exec

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type storeGetKeyParams struct {
	storeName    string
	key          string
	query        string
	defaultValue *string
}

// Static errors for better error handling.
var (
	ErrInvalidPipeParams = errors.New("invalid number of parameters after the pipe")
	ErrInvalidIdentifier = errors.New("invalid identifier after the pipe")
	ErrInvalidParams     = errors.New("invalid number of parameters")
)

// parseStoreGetKeyInput parses the input string and returns storeGetKeyParams.
func parseStoreGetKeyInput(input string) (*storeGetKeyParams, error) {
	// Split the input on the pipe symbol to separate the store parameters and default value.
	parts := strings.Split(input, "|")
	storePart := strings.TrimSpace(parts[0])

	// Default value and query.
	var defaultValue *string
	var query string
	if len(parts) > 1 {
		// Expecting the format: default <value> or query <yq-expression>.
		for _, p := range parts[1:] {
			pipeParts := strings.Fields(strings.TrimSpace(p))
			if len(pipeParts) != 2 {
				return nil, fmt.Errorf("%w: %d", ErrInvalidPipeParams, len(pipeParts))
			}
			v1 := strings.Trim(pipeParts[0], `"'`) // Remove surrounding quotes if present.
			v2 := strings.Trim(pipeParts[1], `"'`)
			switch v1 {
			case "default":
				defaultValue = &v2
			case "query":
				query = v2
			default:
				return nil, fmt.Errorf("%w: %s", ErrInvalidIdentifier, v1)
			}
		}
	}

	// Process the main store part.
	storeParts := strings.Fields(storePart)
	partsLength := len(storeParts)
	if partsLength != 2 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidParams, partsLength)
	}

	return &storeGetKeyParams{
		storeName:    strings.TrimSpace(storeParts[0]),
		key:          strings.TrimSpace(storeParts[1]),
		defaultValue: defaultValue,
		query:        query,
	}, nil
}

func processTagStoreGetKey(atmosConfig *schema.AtmosConfiguration, input string, currentStack string) any {
	log.Debug("Executing Atmos YAML function", function, input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStoreGetKey)
	if err != nil {
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
	}

	// Parse the input parameters.
	retParams, err := parseStoreGetKeyInput(str)
	if err != nil {
		log.Error(invalidYamlFuncMsg, function, input, "error", err)
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
	}

	// Retrieve the store from atmosConfig.
	store := atmosConfig.Stores[retParams.storeName]
	if store == nil {
		log.Error("store not found", function, input, "store", retParams.storeName)
		return fmt.Sprintf("store not found: %s", retParams.storeName)
	}

	// Retrieve the value from the store using the arbitrary key.
	value, err := store.GetKey(retParams.key)
	if err != nil {
		if retParams.defaultValue != nil {
			return *retParams.defaultValue
		}
		log.Error("failed to get key", function, input, "key", retParams.key, "error", err)
		return fmt.Sprintf("failed to get key %s: %v", retParams.key, err)
	}

	// Execute the YQ expression if provided.
	res := value
	if retParams.query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, value, retParams.query)
		if err != nil {
			log.Error("failed to evaluate YQ expression", function, input, "error", err)
			return fmt.Sprintf("failed to evaluate YQ expression: %v", err)
		}
	}

	return res
} 