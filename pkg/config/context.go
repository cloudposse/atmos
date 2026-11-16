package config

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const contextSeparator = "-"

var errContextValueMissing = errors.New("is not provided")

// GetContextFromVars creates a context object from the provided variables.
func GetContextFromVars(vars map[string]any) schema.Context {
	var context schema.Context

	if namespace, ok := vars["namespace"].(string); ok {
		context.Namespace = namespace
	}

	if tenant, ok := vars["tenant"].(string); ok {
		context.Tenant = tenant
	}

	if environment, ok := vars["environment"].(string); ok {
		context.Environment = environment
	}

	if stage, ok := vars["stage"].(string); ok {
		context.Stage = stage
	}

	if region, ok := vars["region"].(string); ok {
		context.Region = region
	}

	if attributes, ok := vars["attributes"].([]any); ok {
		context.Attributes = attributes
	}

	return context
}

// GetContextPrefix calculates context prefix from the context.
func GetContextPrefix(stack string, context *schema.Context, stackNamePattern string, stackFile string) (string, error) {
	if len(stackNamePattern) == 0 {
		return "", errUtils.ErrMissingStackNameTemplateAndPattern
	}

	values := map[string]string{
		"{namespace}": context.Namespace, "{tenant}": context.Tenant,
		"{environment}": context.Environment, "{stage}": context.Stage,
	}
	var parts []string
	for _, token := range strings.Split(stackNamePattern, contextSeparator) {
		value, known := values[token]
		if !known {
			continue
		}
		if value == "" {
			name := strings.Trim(token, "{}")
			article := "a"
			if name == "environment" {
				article = "an"
			}
			return "", fmt.Errorf("%w: the stack name pattern '%s' specifies '%s', but the stack '%s' does not have %s %s defined in the stack file '%s'",
				errUtils.ErrStackNamePatternPartMissing, stackNamePattern, name, stack, article, name, stackFile)
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, contextSeparator), nil
}

// ReplaceContextTokens replaces context tokens in the provided pattern and returns a string with all the tokens replaced.
func ReplaceContextTokens(context *schema.Context, pattern string) string {
	r := strings.NewReplacer(
		"{base-component}", context.BaseComponent,
		"{component}", context.Component,
		"{component-path}", context.ComponentPath,
		"{namespace}", context.Namespace,
		"{environment}", context.Environment,
		"{region}", context.Region,
		"{tenant}", context.Tenant,
		"{stage}", context.Stage,
		"{workspace}", context.Workspace,
		"{terraform_workspace}", context.TerraformWorkspace,
		"{attributes}", strings.Join(u.SliceOfInterfacesToSliceOfStrings(context.Attributes), contextSeparator),
	)
	return r.Replace(pattern)
}

// GetStackNameFromContextAndStackNamePattern calculates stack name from the provided context using the provided stack name pattern.
func GetStackNameFromContextAndStackNamePattern(
	namespace string,
	tenant string,
	environment string,
	stage string,
	stackNamePattern string,
) (string, error) {
	if len(stackNamePattern) == 0 {
		return "", errUtils.ErrMissingStackNameTemplateAndPattern
	}

	values := map[string]string{
		"{namespace}": namespace, "{tenant}": tenant,
		"{environment}": environment, "{stage}": stage,
	}
	var parts []string
	for _, token := range strings.Split(stackNamePattern, contextSeparator) {
		value, known := values[token]
		if !known {
			continue
		}
		if value == "" {
			return "", fmt.Errorf("stack name pattern '%s' includes '%s', but %s %w", stackNamePattern, token,
				strings.Trim(token, "{}"), errContextValueMissing)
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, contextSeparator), nil
}
