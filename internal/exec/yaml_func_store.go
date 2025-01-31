package exec

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type params struct {
	storeName string
	stack     string
	component string
	key       string
}

func getParams(input string, currentStack string) (params, error) {
	parts := strings.Split(input, " ")

	partsLength := len(parts)
	if partsLength != 3 && partsLength != 4 {
		return params{}, fmt.Errorf("invalid Atmos Store YAML function execution:: %s\ninvalid parameters: store_name, {stack}, component, key", input)
	}

	retParams := params{storeName: strings.TrimSpace(parts[0])}

	if partsLength == 4 {
		retParams.stack = strings.TrimSpace(parts[1])
		retParams.component = strings.TrimSpace(parts[2])
		retParams.key = strings.TrimSpace(parts[3])
	} else if partsLength == 3 {
		retParams.stack = currentStack
		retParams.component = strings.TrimSpace(parts[1])
		retParams.key = strings.TrimSpace(parts[2])
	} else {
		return params{}, fmt.Errorf("invalid Atmos Store YAML function execution:: %s\ninvalid parameters: store_name, {stack}, component, key", input)
	}

	return retParams, nil
}

func processTagStore(atmosConfig schema.AtmosConfiguration, input string, currentStack string) any {
	log.Debug("Executing Atmos YAML function store", "input", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStore)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	params, err := getParams(str, currentStack)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	store := atmosConfig.Stores[params.storeName]

	if store == nil {
		u.LogErrorAndExit(fmt.Errorf("invalid Atmos Store YAML function execution:: %s\nstore '%s' not found", input, params.storeName))
	}

	value, err := store.Get(params.stack, params.component, params.key)
	if err != nil {
		u.LogErrorAndExit(fmt.Errorf("an error occurred while looking up key %s in stack %s and component %s from store %s\n%v", params.key, params.stack, params.component, params.storeName, err))
	}

	return value
}
