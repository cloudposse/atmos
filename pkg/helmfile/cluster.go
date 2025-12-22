// Package helmfile provides utilities for helmfile configuration and execution.
package helmfile

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Context is an alias for schema.Context for use in this package.
type Context = schema.Context

// ClusterNameInput contains all possible sources for cluster name resolution.
type ClusterNameInput struct {
	// FlagValue is the value from --cluster-name flag (highest priority).
	FlagValue string
	// ConfigValue is the value from cluster_name in config.
	ConfigValue string
	// Template is the cluster_name_template value (Go template syntax).
	Template string
	// Pattern is the cluster_name_pattern value (deprecated token syntax).
	Pattern string
}

// ClusterNameResult contains the resolved cluster name and metadata.
type ClusterNameResult struct {
	// ClusterName is the resolved cluster name.
	ClusterName string
	// Source indicates where the cluster name came from.
	Source string
	// IsDeprecated is true if the source uses deprecated configuration.
	IsDeprecated bool
}

// TemplateProcessor is a function that processes Go templates.
// This allows injecting the template processor for testing.
type TemplateProcessor func(
	atmosConfig *schema.AtmosConfiguration,
	tmplName string,
	tmplValue string,
	tmplData any,
	ignoreMissingTemplateValues bool,
) (string, error)

// ResolveClusterName determines the EKS cluster name with precedence:
// 1. The --cluster-name flag (highest - always overrides).
// 2. The cluster_name in config.
// 3. The cluster_name_template expanded (Go template syntax).
// 4. The cluster_name_pattern expanded (deprecated, logs warning).
func ResolveClusterName(
	input ClusterNameInput,
	context *Context,
	atmosConfig *schema.AtmosConfiguration,
	componentSection map[string]any,
	templateProcessor TemplateProcessor,
) (*ClusterNameResult, error) {
	defer perf.Track(atmosConfig, "helmfile.ResolveClusterName")()

	// 1. --cluster-name flag (highest priority).
	if input.FlagValue != "" {
		return &ClusterNameResult{
			ClusterName:  input.FlagValue,
			Source:       "flag",
			IsDeprecated: false,
		}, nil
	}

	// 2. cluster_name in config.
	if input.ConfigValue != "" {
		return &ClusterNameResult{
			ClusterName:  input.ConfigValue,
			Source:       "config",
			IsDeprecated: false,
		}, nil
	}

	// 3. cluster_name_template (Go template syntax).
	if input.Template != "" {
		clusterName, err := templateProcessor(atmosConfig, "cluster_name_template", input.Template, componentSection, false)
		if err != nil {
			return nil, fmt.Errorf("failed to process cluster_name_template: %w", err)
		}
		return &ClusterNameResult{
			ClusterName:  clusterName,
			Source:       "template",
			IsDeprecated: false,
		}, nil
	}

	// 4. cluster_name_pattern (deprecated token replacement).
	if input.Pattern != "" {
		clusterName := cfg.ReplaceContextTokens(*context, input.Pattern)
		return &ClusterNameResult{
			ClusterName:  clusterName,
			Source:       "pattern",
			IsDeprecated: true,
		}, nil
	}

	// No cluster name source configured.
	return nil, fmt.Errorf("%w: use --cluster-name flag, or configure cluster_name, cluster_name_template, or cluster_name_pattern",
		errUtils.ErrMissingHelmfileClusterName)
}
