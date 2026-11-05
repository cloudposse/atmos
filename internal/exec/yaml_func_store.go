package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagStore(atmosConfig *schema.AtmosConfiguration, input string, currentStack string) any {
	defer perf.Track(atmosConfig, "exec.processTagStore")()

	log.Debug("Executing Atmos YAML function", "function", input)

	str, err := getStringAfterTag(input, u.AtmosYamlFuncStore)
	errUtils.CheckErrorPrintAndExit(err, "", "")

	_, err = fnparser.ParseStore(str)
	if err != nil {
		log.Error(invalidYamlFuncMsg, function, input, "error", err)
		return fmt.Sprintf("%s: %s", invalidYamlFuncMsg, input)
	}
	// Use the shared scoped read for secret-store refusal, authentication, value defaults,
	// and query evaluation. Authentication failures must never activate a value default.
	value, err := storedeferred.ReadStore(atmosConfig, input, currentStack, nil)
	errUtils.CheckErrorPrintAndExit(err, "", "")
	return value
}
