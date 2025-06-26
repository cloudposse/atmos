package exec

import (
	"encoding/json"
	"fmt"
	atmoserr "github.com/cloudposse/atmos/errors"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagTemplate(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	u.LogDebug(fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, u.AtmosYamlFuncTemplate)
	if err != nil {
		atmoserr.PrintErrorMarkdownAndExit(err, "", "")
	}

	var decoded any
	if err = json.Unmarshal([]byte(str), &decoded); err != nil {
		return str
	}

	return decoded
}
