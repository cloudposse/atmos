package utils

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

func SetEnvironmentVariable(stackInfo *schema.ConfigAndStacksInfo, key, value string) {
	if stackInfo == nil || stackInfo.ComponentEnvSection == nil {
		stackInfo.ComponentEnvSection = schema.AtmosSectionMapType{}
	}
	stackInfo.ComponentEnvSection[key] = value
	stackInfo.ComponentEnvList = append(stackInfo.ComponentEnvList, fmt.Sprintf("%s=%s", key, value))
}
