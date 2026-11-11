package exec

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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

	_, err = fnparser.ParseStoreGet(str)
	if err != nil {
		log.Error(invalidYamlFuncMsg, function, input, "error", err)
		var parseErr *fnparser.Error
		if errors.As(err, &parseErr) && parseErr.Message == "expected option value" {
			return fmt.Sprintf("invalid parameters after pipe: %s", input)
		}
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
	}
	// The shared scoped read retrieves an arbitrary key, applies value defaults and
	// evaluates a query after releasing the authentication lock. Identity errors and
	// denied access never activate a default, including on this legacy path.
	value, err := storedeferred.ReadStore(atmosConfig, u.AtmosYamlFuncStoreGet+" "+str, currentStack, nil)
	errUtils.CheckErrorPrintAndExit(err, "", "")
	return value
}
