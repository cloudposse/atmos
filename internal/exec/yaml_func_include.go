package exec

import (
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func processTagInclude(
	atmosConfig schema.AtmosConfiguration,
	input string,
	fileType string,
) any {
	str, err := getStringAfterTag(input, fileType)
	if err != nil {
		log.Fatal(err)
	}

	log.Debug("Executing Atmos YAML function", "!include", str)

	var f string
	q := ""

	parts, err := u.SplitStringByDelimiter(str, ' ')
	if err != nil {
		e := fmt.Errorf("error executing the YAML function: !include %s\n%v", str, err)
		log.Fatal(e)
	}

	partsLen := len(parts)

	if partsLen == 2 {
		f = strings.TrimSpace(parts[0])
		q = strings.TrimSpace(parts[1])
	} else if partsLen == 1 {
		f = strings.TrimSpace(parts[0])
	} else {
		err = fmt.Errorf("invalid number of arguments in the Atmos YAML function: !include %s. The function accepts 1 or 2 arguments", str)
		log.Fatal(err)
	}

	var res any

	if fileType == u.AtmosYamlFuncIncludeLocalFile {
		res, err = u.DetectFormatAndParseFile(f)
	} else if fileType == u.AtmosYamlFuncIncludeGoGetter {
		res, err = DownloadDetectFormatAndParseFile(&atmosConfig, f)
	}

	if err != nil {
		e := fmt.Errorf("error evaluating the YAML function: !include %s\n%v", str, err)
		log.Fatal(e)
	}

	if q != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, res, q)
		if err != nil {
			log.Fatal(err)
		}
	}

	return res
}
