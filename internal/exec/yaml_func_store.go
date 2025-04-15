package exec

import (
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"

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

const (
	invalidYamlFuncMsg = "invalid YAML function"
	function           = "function"
)

func processTagStore(atmosConfig schema.AtmosConfiguration, input string, currentStack string) any {
	log.Debug("Executing Atmos YAML function", function, input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStore)
	if err != nil {
		log.Fatal(err)
	}

	// Split the input on the pipe symbol to separate the store parameters and default value
	parts := strings.Split(str, "|")
	storePart := strings.TrimSpace(parts[0])

	// Default value and query
	var defaultValue *string
	var query string
	if len(parts) > 1 {
		// Expecting the format: default <value> or query <yq-expression>
		for _, p := range parts[1:] {
			pipeParts := strings.Fields(strings.TrimSpace(p))
			if len(pipeParts) != 2 {
				log.Error(invalidYamlFuncMsg, function, input, "invalid number of parameters after the pipe", len(pipeParts))
				return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
			}
			v1 := strings.Trim(pipeParts[0], `"'`) // Remove surrounding quotes if present
			v2 := strings.Trim(pipeParts[1], `"'`)
			switch v1 {
			case "default":
				defaultValue = &v2
			case "query":
				query = v2
			default:
				log.Error(invalidYamlFuncMsg, function, input, "invalid identifier after the pipe", v1)
				return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
			}
		}
	}

	// Process the main store part
	storeParts := strings.Fields(storePart)
	partsLength := len(storeParts)
	if partsLength != 3 && partsLength != 4 {
		log.Error(invalidYamlFuncMsg, function, input, "invalid number of parameters", partsLength)
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
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

	// Retrieve the store from atmosConfig
	store := atmosConfig.Stores[retParams.storeName]

	if store == nil {
		log.Fatal("store not found", function, input, "store", retParams.storeName)
	}

	// Retrieve the value from the store
	value, err := store.Get(retParams.stack, retParams.component, retParams.key)
	if err != nil {
		if retParams.defaultValue != nil {
			return *retParams.defaultValue
		}
		log.Fatal("failed to get key", function, input, "key", retParams.key, "error", err)
	}

	// Execute the YQ expression if provided
	res := value

	if retParams.query != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, value, retParams.query)
		if err != nil {
			log.Fatal(err)
		}
	}

	return res
}
