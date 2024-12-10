package exec

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagStore(cliConfig schema.CliConfiguration, input string) any {
	log.Info("Executing Atmos YAML function store", "input", input)

	str, err := getStringAfterTag(cliConfig, input, u.AtmosYamlFuncStore)

	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	parts := strings.Split(str, " ")
	if len(parts) != 2 {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos Store YAML function execution:: %s\nexactly two parameters are required: store_name, key", input))
	}

	storeName := strings.TrimSpace(parts[0])
	key := strings.TrimSpace(parts[1])

	store := cliConfig.Stores[storeName]
	log.Info("stores", "stores", cliConfig.Stores)

	if store == nil {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos Store YAML function execution:: %s\nstore '%s' not found", input, storeName))
	}

	value, err := store.Get(key)
	if err != nil {
		u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos Store YAML function execution:: %s\nkey '%s' not found in store '%s'", input, key, storeName))
	}

	return value
}
