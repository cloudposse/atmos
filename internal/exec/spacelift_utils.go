package exec

import (
	"fmt"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// BuildSpaceliftStackName builds a Spacelift stack name from the provided context and stack name pattern
func BuildSpaceliftStackName(spaceliftSettings map[any]any, context cfg.Context, contextPrefix string) (string, string) {
	if spaceliftStackNamePattern, ok := spaceliftSettings["stack_name_pattern"].(string); ok {
		return cfg.ReplaceContextTokens(context, spaceliftStackNamePattern), spaceliftStackNamePattern
	} else if spaceliftStackName, ok := spaceliftSettings["stack_name"].(string); ok {
		return spaceliftStackName, contextPrefix
	} else {
		defaultSpaceliftStackNamePattern := fmt.Sprintf("%s-%s", contextPrefix, context.Component)
		return strings.Replace(defaultSpaceliftStackNamePattern, "/", "-", -1), contextPrefix
	}
}
