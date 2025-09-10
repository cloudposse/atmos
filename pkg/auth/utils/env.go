package utils

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

// SetEnvironmentVariable appends an environment variable to the stack info map/list.
func SetEnvironmentVariable(stackInfo *schema.ConfigAndStacksInfo, key, value string) {
	if stackInfo == nil {
		return
	}
	if stackInfo.ComponentEnvSection == nil {
		stackInfo.ComponentEnvSection = schema.AtmosSectionMapType{}
	}
	stackInfo.ComponentEnvSection[key] = value
	stackInfo.ComponentEnvList = append(stackInfo.ComponentEnvList, fmt.Sprintf("%s=%s", key, value))
}
