package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
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

	parsed, err := fnparser.ParseStoreGet(str)
	if err != nil {
		log.Error(invalidYamlFuncMsg, function, input, "error", err)
		if strings.Contains(err.Error(), "expected option value") {
			return fmt.Sprintf("invalid parameters after pipe: %s", input)
		}
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
	}
	retParams := getKeyParams{
		storeName:    parsed.Store,
		key:          parsed.Key,
		defaultValue: parsed.Default,
		query:        parsed.Query,
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
		er := fmt.Errorf("%w: failed to execute YAML function %s for key %s: %s", ErrGetKeyFailed, input, retParams.key, err)
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
