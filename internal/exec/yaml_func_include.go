package exec

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/downloader"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagInclude(
	atmosConfig schema.AtmosConfiguration,
	input string,
	fileType string,
	currentStack string,
) any {
	str, err := getStringAfterTag(input, fileType)
	if err != nil {
		u.LogErrorAndExit(err)
	}

	u.LogTrace(fmt.Sprintf("Executing Atmos YAML function: !include %s", str))

	var f string
	q := ""

	parts, err := u.SplitStringByDelimiter(str, ' ')
	if err != nil {
		e := fmt.Errorf("error executing the YAML function: !include %s\n%v", str, err)
		u.LogErrorAndExit(e)
	}

	partsLen := len(parts)

	if partsLen == 2 {
		f = strings.TrimSpace(parts[0])
		q = strings.TrimSpace(parts[1])
	} else if partsLen == 1 {
		f = strings.TrimSpace(parts[0])
	} else {
		err = fmt.Errorf("invalid number of arguments in the Atmos YAML function: !include %s. The function accepts 1 or 2 arguments", str)
		u.LogErrorAndExit(err)
	}

	var res any

	if fileType == u.AtmosYamlFuncIncludeLocalFile {
		res, err = u.DetectFormatAndParseFile(f)
	} else if fileType == u.AtmosYamlFuncIncludeGoGetter {
		res, err = downloader.NewGoGetterDownloader(&atmosConfig).FetchAndAutoParse(f)
	}

	if err != nil {
		e := fmt.Errorf("error evaluating the YAML function: !include %s\n%v", str, err)
		u.LogErrorAndExit(e)
	}

	if q != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, res, q)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	}

	return res
}
