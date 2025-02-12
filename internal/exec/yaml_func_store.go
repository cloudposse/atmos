package exec

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type params struct {
	storeName    string
	stack        string
	component    string
	key          string
	defaultValue *string
}

func processTagStore(atmosConfig schema.AtmosConfiguration, input string, currentStack string) any {
	log.Debug("Executing Atmos YAML function store", "input", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStore)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	// Split the input on the pipe symbol to separate the store parameters and default value
	parts := strings.Split(str, "|")
	storePart := strings.TrimSpace(parts[0])

	var defaultValue *string
	if len(parts) > 1 {
		// Expecting the format: default <value>
		defaultParts := strings.Fields(strings.TrimSpace(parts[1]))
		if len(defaultParts) != 2 || defaultParts[0] != "default" {
			log.Error(fmt.Sprintf("invalid default value format in: %s", str))
			return fmt.Sprintf("invalid default value format in: %s", str)
		}
		val := strings.Trim(defaultParts[1], `"'`) // Remove surrounding quotes if present
		defaultValue = &val
	}

	// Process the main store part
	storeParts := strings.Fields(storePart)
	partsLength := len(storeParts)
	if partsLength != 3 && partsLength != 4 {
		return fmt.Sprintf("invalid Atmos Store YAML function execution:: %s\ninvalid parameters: store_name, {stack}, component, key", input)
	}

	retParams := params{
		storeName:    strings.TrimSpace(storeParts[0]),
		defaultValue: defaultValue,
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
		u.LogErrorAndExit(fmt.Errorf("invalid Atmos Store YAML function execution:: %s\nstore '%s' not found", input, retParams.storeName))
	}

	// Retrieve the value from the store
	value, err := store.Get(retParams.stack, retParams.component, retParams.key)
	if err != nil {
		if retParams.defaultValue != nil {
			return *retParams.defaultValue
		}
		u.LogErrorAndExit(fmt.Errorf("failed to get key: %s", err))
	}

	return value
}
