package naming

import (
	"fmt"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TemplateEvaluator renders a naming expression using the caller's template engine.
type TemplateEvaluator func(*schema.AtmosConfiguration, string, string, any, bool) (string, error)

// Resolver shares runtime naming precedence, normalization, and context construction.
// StackTemplate must also reject computed stack identities while evaluating expressions.
// The evaluator is injected so this package does not depend on command execution.
type Resolver struct {
	Template      TemplateEvaluator
	StackTemplate TemplateEvaluator
}

// ResolveStackName returns the logical name and the context populated by a legacy pattern.
// Precedence is manifest name, template, pattern, then filename. Invalid patterns fall back to filename.
func (r Resolver) ResolveStackName(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName, stackManifestName string,
	info schema.ConfigAndStacksInfo, //nolint:gocritic // hugeParam: read-only, passed by value intentionally.
	varsSection map[string]any,
) (string, schema.Context, error) {
	defer perf.Track(atmosConfig, "naming.ResolveStackName")()

	switch {
	case stackManifestName != "":
		return stackManifestName, schema.Context{}, nil

	case atmosConfig.Stacks.NameTemplate != "":
		name, err := r.StackTemplate(atmosConfig, stackFileName,
			atmosConfig.Stacks.NameTemplate, info.ComponentSection, false)
		if err != nil {
			return "", schema.Context{}, err
		}
		return name, schema.Context{}, nil

	case atmosConfig.Stacks.NamePattern != "":
		context := cfg.GetContextFromVars(varsSection)
		name, err := cfg.GetContextPrefix(stackFileName, context, atmosConfig.Stacks.NamePattern, stackFileName)
		if err != nil {
			// Fall back to filename when pattern validation fails.
			log.Debug("Pattern validation failed, using filename as stack name",
				"stack", stackFileName, "error", err)
			return stackFileName, context, nil
		}
		return name, context, nil

	default:
		return stackFileName, schema.Context{}, nil
	}
}

// ProcessStackContextPrefix populates command context and rejects invalid stack identities.
func (r Resolver) ProcessStackContextPrefix(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	stackName string,
	stackManifestName string,
) error {
	defer perf.Track(atmosConfig, "naming.ProcessStackContextPrefix")()

	// If stack has explicit name, skip template/pattern processing.
	// The explicit name takes precedence over any generated name.
	// Still populate Context from vars to avoid stale values from previous iterations.
	if stackManifestName != "" {
		if err := EnsureLiteralStackIdentity(stackName, stackManifestName, configAndStacksInfo.ComponentSection); err != nil {
			return err
		}
		configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		configAndStacksInfo.ContextPrefix = stackManifestName
		configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
		configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath
		return nil
	}

	switch {
	case atmosConfig.Stacks.NameTemplate != "":
		tmpl, err := r.StackTemplate(atmosConfig, stackName, atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
		if err != nil {
			return err
		}
		configAndStacksInfo.ContextPrefix = tmpl
	case atmosConfig.Stacks.NamePattern != "":
		// Process context.
		configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)

		var err error
		configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(
			configAndStacksInfo.Stack,
			configAndStacksInfo.Context,
			atmosConfig.Stacks.NamePattern,
			stackName,
		)
		if err != nil {
			return err
		}
	default:
		// No name_template or name_pattern configured - use filename as identity.
		// This enables zero-config stack naming for newcomers.
		configAndStacksInfo.ContextPrefix = stackName
	}

	if err := EnsureLiteralStackIdentity(stackName, configAndStacksInfo.ContextPrefix, configAndStacksInfo.ComponentSection); err != nil {
		return err
	}

	configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
	configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath
	return nil
}

// BuildTerraformWorkspace resolves workspace overrides and normalizes slash separators.
func (r Resolver) BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "naming.BuildTerraformWorkspace")()

	// Return 'default' workspace if workspaces are disabled.
	// Terraform always operates in the `default` workspace when multiple workspaces are unsupported or disabled,
	// preventing switching or creating additional workspaces.
	if !WorkspacesEnabled(atmosConfig, configAndStacksInfo) {
		return cfg.TerraformDefaultWorkspace, nil
	}

	contextPrefix, err := r.workspaceContextPrefix(atmosConfig, configAndStacksInfo)
	if err != nil {
		return "", err
	}
	workspace, err := r.workspaceOverride(atmosConfig, configAndStacksInfo, contextPrefix)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(workspace, "/", "-"), nil
}

// workspaceContextPrefix preserves workspace-specific fallback and missing-value behavior.
func (r Resolver) workspaceContextPrefix(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	// Stack name precedence: name (from manifest) > name_template > name_pattern > filename.
	switch {
	case info.StackManifestName != "":
		return info.StackManifestName, nil
	case config.Stacks.NameTemplate != "":
		return r.StackTemplate(config, info.Stack, config.Stacks.NameTemplate, info.ComponentSection, config.Templates.Settings.IgnoreMissingTemplateValues)
	case config.Stacks.NamePattern != "":
		prefix, err := cfg.GetContextPrefix(info.Stack, info.Context, config.Stacks.NamePattern, info.Stack)
		if err == nil {
			return prefix, nil
		}
		// Fall back to filename when pattern validation fails.
	}
	return strings.ReplaceAll(info.Stack, "/", "-"), nil
}

func (r Resolver) workspaceOverride(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prefix string) (string, error) {
	metadata := info.ComponentMetadataSection
	// Terraform workspace can be overridden per component using metadata.terraform_workspace_template,
	// metadata.terraform_workspace_pattern, or metadata.terraform_workspace, in that order.
	if value, ok := metadata["terraform_workspace_template"].(string); ok {
		return r.Template(config, "terraform-workspace-template", value, info.ComponentSection, false)
	}
	if value, ok := metadata["terraform_workspace_pattern"].(string); ok {
		return cfg.ReplaceContextTokens(info.Context, value), nil
	}
	if value, ok := metadata["terraform_workspace"].(string); ok {
		return value, nil
	}
	if info.Context.BaseComponent == "" {
		return prefix, nil
	}
	return fmt.Sprintf("%s-%s", prefix, info.Context.Component), nil
}

// WorkspacesEnabled reports whether the configuration and backend support workspaces.
func WorkspacesEnabled(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	defer perf.Track(atmosConfig, "naming.WorkspacesEnabled")()

	// Check if using HTTP backend first, as it doesn't support workspaces.
	if info.ComponentBackendType == "http" {
		// If workspaces are explicitly enabled for HTTP backend, log a warning.
		if atmosConfig.Components.Terraform.WorkspacesEnabled != nil && *atmosConfig.Components.Terraform.WorkspacesEnabled {
			log.Warn("ignoring unsupported workspaces `enabled` setting for HTTP backend type.",
				"backend", "http",
				"component", info.Component)
		}
		return false
	}

	// Check if workspaces are explicitly disabled.
	if atmosConfig.Components.Terraform.WorkspacesEnabled != nil && !*atmosConfig.Components.Terraform.WorkspacesEnabled {
		return false
	}

	return true
}
