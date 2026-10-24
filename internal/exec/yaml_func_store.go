package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
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

func processTagStore(atmosConfig *schema.AtmosConfiguration, input string, currentStack string) any {
	defer perf.Track(atmosConfig, "exec.processTagStore")()

	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStore)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	parsed, err := fnparser.ParseStore(str)
	if err != nil {
		log.Error(invalidYamlFuncMsg, function, input, "error", err)
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
	}
	retParams := params{
		storeName:    parsed.Store,
		stack:        parsed.Stack,
		component:    parsed.Component,
		key:          parsed.Key,
		defaultValue: parsed.Default,
		query:        parsed.Query,
	}
	if retParams.stack == "" {
		retParams.stack = currentStack
	}

	// Refuse !store access to a secret store; secret stores are only reachable via !secret.
	if cfg, ok := atmosConfig.StoresConfig[retParams.storeName]; ok && cfg.Secret {
		er := fmt.Errorf("%w: store %q (in %s)", errUtils.ErrStoreIsSecret, retParams.storeName, input)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// Retrieve the store from atmosConfig
	store := atmosConfig.Stores[retParams.storeName]

	if store == nil {
		er := fmt.Errorf("failed to execute YAML function %s. Store %s not found", input, retParams.storeName)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// Retrieve the value from the store
	value, err := store.Get(retParams.stack, retParams.component, retParams.key)
	if err != nil {
		if retParams.defaultValue != nil {
			return *retParams.defaultValue
		}
		er := fmt.Errorf("error executing YAML function %s. Failed to get key %s. Error: %w", input, retParams.key, err)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// Execute the YQ expression if provided
	res := value

	if retParams.query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, value, retParams.query)
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	return res
}
