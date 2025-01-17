package exec

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagInclude(
	atmosConfig schema.AtmosConfiguration,
	input string,
	currentStack string,
) any {
	u.LogTrace(atmosConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(input, config.AtmosYamlFuncInclude)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	// Split the value into slices based on any whitespace (one or more spaces, tabs, or newlines),
	// while also ignoring leading and trailing whitespace
	var f string
	q := ""
	parts := strings.Fields(str)
	partsLen := len(parts)

	if partsLen == 2 {
		f = strings.TrimSpace(parts[0])
		q = strings.TrimSpace(parts[1])
	} else if partsLen == 1 {
		f = strings.TrimSpace(parts[0])
	} else {
		err = fmt.Errorf("invalid number of arguments in the Atmos YAML function: !include %s. The function accepts 1 or 2 arguments", input)
		u.LogErrorAndExit(atmosConfig, err)
	}

	var res any
	err = u.DetectFormatAndParseFile(f, &res)
	if err != nil {
		u.LogErrorAndExit(atmosConfig, err)
	}

	if q != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, res, q)
		if err != nil {
			u.LogErrorAndExit(atmosConfig, err)
		}
	}

	return res
}
