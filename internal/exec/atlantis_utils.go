package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// BuildAtlantisProjectName builds an Atlantis project name from the provided context and project name pattern
func BuildAtlantisProjectName(context cfg.Context, projectNameTemplate string) string {
	return cfg.ReplaceContextTokens(context, projectNameTemplate)
}
