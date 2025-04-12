package exec

import (
	"errors"
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

var (
	ErrInvalidYamlFuncStore = errors.New("invalid YAML function")
)

func processTagStore(atmosConfig schema.AtmosConfiguration, input string, currentStack string) any {
	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStore)
	if err != nil {
		log.Fatal(err)
	}

	// Split the input on the pipe symbol to separate the store parameters and default value
	parts := strings.Split(str, "|")
	storePart := strings.TrimSpace(parts[0])

	var defaultValue *string
	var query string
	if len(parts) > 1 {
		// Expecting the format: default <value> or query <yq-expression>
		for _, p := range parts[1:] {
			pipeParts := strings.Fields(strings.TrimSpace(p))
			if len(pipeParts) != 2 {
				e := fmt.Errorf("%w: %s", ErrInvalidYamlFuncStore, input)
				log.Error(e)
				return fmt.Sprintf("invalid YAML function: %s", input)
			}
			v1 := strings.Trim(pipeParts[0], `"'`) // Remove surrounding quotes if present
			v2 := strings.Trim(pipeParts[1], `"'`) // Remove surrounding quotes if present
			switch v1 {
			case "default":
				defaultValue = &v2
			case "query":
				query = v2
			default:
				e := fmt.Errorf("%w: %s", ErrInvalidYamlFuncStore, input)
				log.Error(e)
				return fmt.Sprintf("invalid YAML function: %s", input)
			}
		}
	}

	// Process the main store part
	storeParts := strings.Fields(storePart)
	partsLength := len(storeParts)
	if partsLength != 3 && partsLength != 4 {
		e := fmt.Errorf("%w: %s", ErrInvalidYamlFuncStore, input)
		log.Error(e)
		return fmt.Sprintf("invalid YAML function: %s", input)
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
		log.Fatal(fmt.Errorf("%w: %s\nstore '%s' not found", ErrInvalidYamlFuncStore, input, retParams.storeName))
	}

	// Retrieve the value from the store
	value, err := store.Get(retParams.stack, retParams.component, retParams.key)
	if err != nil {
		if retParams.defaultValue != nil {
			return *retParams.defaultValue
		}
		log.Fatal(fmt.Errorf("%w: %s\nfailed to get key: %s\nerror: %v", ErrInvalidYamlFuncStore, input, retParams.key, err))
	}

	// Execute the YQ expression
	var res any

	if retParams.query != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, value, retParams.query)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		res = value
	}

	return res
}
