package cloudformation

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// packagingOperations are the operations that send the template to the API and
// therefore package an oversized template through a `kind: aws/s3` target first.
var packagingOperations = map[Operation]bool{
	OperationApply:           true,
	OperationDiff:            true,
	OperationValidate:        true,
	OperationChangesetCreate: true,
}

// validateDryRun validates known values without evaluating deferred expressions.
// Copies are used only for static validation; no placeholder reaches an API.
// It applies the same static rules the real run enforces before any mutation
// (packaging-target resolution, StackSet target shape), so a dry run does not
// report a configuration valid that the real run would reject.
func validateDryRun(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, flags map[string]any, operation Operation) error {
	section := maps.Clone(info.ComponentSection)
	if body, ok := section["template"].(string); ok && deferredExpression(atmosConfig, body) {
		section["template"] = "Resources: {}"
	}
	if err := validateComponentConfig(section); err != nil {
		return err
	}
	spec, err := buildStackSpec(section)
	if err != nil {
		return err
	}
	spec.withAtmosIdentity(info)
	check := &dryRunCheck{AtmosConfig: atmosConfig, Info: info, Section: section, Flags: flags, Operation: operation, Spec: spec}
	if err := check.validateStackSet(); err != nil {
		return err
	}
	if err := check.validateTargetAuth(); err != nil {
		return err
	}
	if err := check.validatePackaging(); err != nil {
		return err
	}
	ui.Info(fmt.Sprintf("Dry run: %s %s in %s: configuration validated; no AWS calls made", operation, dryRunComponentName(info), info.Stack))
	return nil
}

// dryRunCheck carries the static inputs shared by the dry-run validators.
type dryRunCheck struct {
	AtmosConfig *schema.AtmosConfiguration
	Info        *schema.ConfigAndStacksInfo
	// Section is a copy of the component section with deferred values neutralized.
	Section   map[string]any
	Flags     map[string]any
	Operation Operation
	Spec      *stackSpec
}

// dryRunComponentName returns the component name for dry-run output.
func dryRunComponentName(info *schema.ConfigAndStacksInfo) string {
	if info.ComponentFromArg != "" {
		return info.ComponentFromArg
	}
	return info.Component
}

// validateStackSet validates the selected StackSet target the way the real
// run does before it creates anything: the target must resolve, permission_model
// must name a known model (otherwise only AWS rejects it), and SERVICE_MANAGED
// cannot be combined with explicit accounts and regions on create.
func (c *dryRunCheck) validateStackSet() error {
	if c.Operation != OperationStackSetCreate && c.Operation != OperationStackSetUpdate {
		return nil
	}
	provision := staticStackSetProvision(c.AtmosConfig, c.Section)
	flagTarget, _ := c.Flags[targetKey].(string)
	ssCfg, err := resolveStackSetTarget(provision, flagTarget)
	if err != nil {
		return err
	}
	if !deferredExpression(c.AtmosConfig, ssCfg.PermissionModel) {
		if err := validateStackSetPermissionModel(ssCfg.Name, ssCfg.PermissionModel); err != nil {
			return err
		}
	}
	if c.Operation == OperationStackSetCreate && len(ssCfg.Accounts) > 0 && len(ssCfg.Regions) > 0 {
		return validateStackSetInstancePermissionModel(cfntypes.PermissionModels(ssCfg.PermissionModel))
	}
	return nil
}

// validateStackSetPermissionModel rejects a permission_model that is not a model
// CloudFormation knows, so a typo fails locally instead of at the AWS API.
func validateStackSetPermissionModel(targetName, model string) error {
	known := cfntypes.PermissionModels("").Values()
	names := make([]string, 0, len(known))
	for _, value := range known {
		if string(value) == model {
			return nil
		}
		names = append(names, string(value))
	}
	return errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
		WithExplanationf("provision.targets.%s.permission_model %q is not a known StackSet permission model.", targetName, model).
		WithHintf("Set permission_model to one of: %s.", strings.Join(names, ", ")).
		Err()
}

// validatePackaging applies the packaging-target rules of the real run.
// An oversized template, or an aws/s3 target selected directly, needs a
// resolvable `kind: aws/s3` target with a bucket and region; apply also rejects
// an unknown --target. The template size is known statically for inline
// templates and for template files that already exist on disk.
func (c *dryRunCheck) validatePackaging() error {
	if !packagingOperations[c.Operation] {
		return nil
	}
	oversized := dryRunTemplateOversized(c.AtmosConfig, c.Info, c.Spec)
	if c.Operation != OperationApply && !oversized {
		return nil
	}
	provision, _ := c.Section[cfg.ProvisionSectionName].(map[string]any)
	flagTarget, _ := c.Flags[targetKey].(string)
	selected, err := target.SelectTargetWithDefault(provision, flagTarget, "default", cfg.CloudFormationComponentType)
	if err != nil {
		return err
	}
	// The real apply rejects a `kind: aws/stackset` target before packaging
	// (see deliverApply), so the dry run must reject it too.
	if c.Operation == OperationApply && selected.Kind == kindAwsStackSet {
		return stackSetTargetError(selected.Name, c.Spec)
	}
	if !oversized && selected.Kind != kindAwsS3 {
		return nil
	}
	_, err = resolvePackagingTarget(provision, selected)
	return err
}

// dryRunTemplateOversized reports whether the template is known, without any
// API call or source provisioning, to exceed the inline size limit. A template
// that cannot be located statically (deferred expression, source not yet
// pulled) is treated as within the limit.
func dryRunTemplateOversized(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, spec *stackSpec) bool {
	if spec.TemplateBody != "" {
		return needsPackaging(spec.TemplateBody)
	}
	if spec.TemplatePath == "" {
		return false
	}
	componentPath := ""
	if !filepath.IsAbs(spec.TemplatePath) {
		var err error
		if componentPath, err = resolveComponentPath(atmosConfig, info); err != nil {
			return false
		}
	}
	stat, err := os.Stat(resolveTemplateFilePath(componentPath, spec))
	return err == nil && stat.Size() > templateInlineSizeLimit
}

// deferredExpression recognizes configured template delimiters and Atmos YAML tags without parsing or
// evaluating their contents.
func deferredExpression(config *schema.AtmosConfiguration, value string) bool {
	left, right := "{{", "}}"
	if config != nil && len(config.Templates.Settings.Delimiters) == 2 {
		left, right = config.Templates.Settings.Delimiters[0], config.Templates.Settings.Delimiters[1]
	}
	// Recognize syntax without parsing functions or evaluating external reads.
	if left != "" && right != "" && strings.Contains(value, left) && strings.Contains(value, right) {
		return true
	}
	value = strings.TrimSpace(value)
	for _, tag := range u.AtmosYamlTags {
		if value == tag || strings.HasPrefix(value, tag+" ") {
			return true
		}
	}
	return false
}

// staticTargetValues omits deferred target expressions from dry-run validation while retaining static
// values, including malformed ones.
func staticTargetValues(config *schema.AtmosConfiguration, value any) any {
	if text, ok := value.(string); ok && deferredExpression(config, text) {
		return nil
	}
	if items, ok := value.([]any); ok {
		result := make([]any, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok && deferredExpression(config, text) {
				continue
			}
			result = append(result, item)
		}
		return result
	}
	return value
}

// staticStackSetProvision copies target maps before removing deferred account and region expressions,
// preserving the caller's configuration.
func staticStackSetProvision(atmosConfig *schema.AtmosConfiguration, section map[string]any) map[string]any {
	provision, _ := section["provision"].(map[string]any)
	rawTargets, _ := provision["targets"].(map[string]any)
	targets := maps.Clone(rawTargets)
	for name, raw := range targets {
		target, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		target = maps.Clone(target)
		for _, field := range []string{"accounts", "regions"} {
			target[field] = staticTargetValues(atmosConfig, target[field])
		}
		targets[name] = target
	}
	return map[string]any{"targets": targets}
}
