package cloudformation

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Field names of one `parameters:` list entry (the AWS CLI / Rain form).
const (
	paramKeyField         = "ParameterKey"
	paramValueField       = "ParameterValue"
	paramUsePreviousField = "UsePreviousValue"
)

// stackSpec is the fully-resolved, SDK-ready shape of an aws/cloudformation component,
// extracted from the component's stack config section.
type stackSpec struct {
	StackName       string
	TemplatePath    string
	TemplateAbsPath string
	TemplateBody    string
	// TemplateURL is set by packageIfNeeded when the template was packaged to a
	// `kind: aws/s3` target (either because it exceeds CloudFormation's
	// 51,200-byte inline TemplateBody limit, or because packaging was
	// otherwise selected). When set, createChangeSet sends TemplateURL
	// instead of TemplateBody -- CreateChangeSet accepts exactly one of the
	// two, and only TemplateURL supports templates over the inline limit.
	TemplateURL           string
	Parameters            []cfntypes.Parameter
	Capabilities          []cfntypes.Capability
	Tags                  []cfntypes.Tag
	StackPolicyFile       string
	StackPolicyBody       string
	RoleArn               string
	NotificationArns      []string
	DisableRollback       bool
	TerminationProtection bool
	TimeoutInMinutes      int32
	// Component and AtmosStack identify the Atmos component and stack the spec
	// was built for, so error hints can name a runnable command. Both are set by
	// the caller; buildStackSpec only sees the component section.
	Component  string
	AtmosStack string
}

// commandTarget returns the `<component> -s <stack>` arguments that address this
// spec's component on the command line, falling back to placeholders for any
// part the caller did not record.
func (s *stackSpec) commandTarget() string {
	component, stack := s.Component, s.AtmosStack
	if component == "" {
		component = "<component>"
	}
	if stack == "" {
		stack = "<stack>"
	}
	return component + " -s " + stack
}

// withAtmosIdentity records the Atmos component and stack the spec belongs to.
func (s *stackSpec) withAtmosIdentity(info *schema.ConfigAndStacksInfo) *stackSpec {
	if info != nil {
		s.Component = info.ComponentFromArg
		s.AtmosStack = info.Stack
	}
	return s
}

// buildStackSpec extracts and normalizes an aws/cloudformation component's
// first-class sections into an SDK-ready spec. TemplateBody is populated by the
// caller after resolving/reading the template file (see template.go).
func buildStackSpec(componentSection map[string]any) (*stackSpec, error) {
	defer perf.Track(nil, "cloudformation.buildStackSpec")()

	stackName, _ := componentSection[cfg.StackNameSectionName].(string)
	if stackName == "" {
		return nil, errUtils.ErrMissingAwsCloudFormationStackName
	}

	templateBody, templatePath, err := resolveTemplateSection(componentSection, stackName)
	if err != nil {
		return nil, err
	}
	if templateBody == "" && templatePath == "" && !isAbstractComponent(componentSection) && !source.HasSource(componentSection) {
		return nil, errUtils.ErrMissingAwsCloudFormationTemplate
	}

	spec := &stackSpec{
		StackName:    stackName,
		TemplatePath: templatePath,
		TemplateBody: templateBody,
	}

	if err := normalizeParametersAndCapabilities(spec, componentSection); err != nil {
		return nil, err
	}

	spec.Tags = normalizeTags(componentSection[cfg.TagsSectionName])

	if stackPolicy, ok := componentSection[cfg.StackPolicySectionName].(map[string]any); ok {
		spec.StackPolicyFile, _ = stackPolicy["file"].(string)
	}

	spec.RoleArn, _ = componentSection[cfg.RoleArnSectionName].(string)
	spec.NotificationArns = normalizeStringSlice(componentSection[cfg.NotificationArnsSectionName])
	spec.DisableRollback, _ = componentSection[cfg.DisableRollbackSectionName].(bool)
	spec.TerminationProtection, _ = componentSection[cfg.TerminationProtectionSectionName].(bool)

	if timeout, ok := componentSection[cfg.TimeoutInMinutesSectionName]; ok {
		// CreateChangeSet/ExecuteChangeSet have no timeout parameter.
		ui.Warning("timeout_in_minutes is unsupported by changeset-based operations and is ignored.")
		spec.TimeoutInMinutes = toInt32(timeout)
	}

	return spec, nil
}

// normalizeParametersAndCapabilities fills the spec's parameters and capabilities,
// failing on a malformed `parameters:` shape or an unknown capability instead of
// letting the deploy proceed with defaults or fail late in the AWS API.
func normalizeParametersAndCapabilities(spec *stackSpec, componentSection map[string]any) error {
	params, err := normalizeParameters(componentSection[cfg.ParametersSectionName])
	if err != nil {
		return err
	}
	spec.Parameters = params

	capabilities, err := normalizeCapabilities(componentSection[cfg.CapabilitiesSectionName])
	if err != nil {
		return err
	}
	spec.Capabilities = capabilities
	return nil
}

// resolveTemplateSection reads the component's `template`/`path` keys and
// returns the resolved inline template body and/or file path. `template` is
// polymorphic: a string is used as the body almost verbatim (a literal
// YAML/JSON block scalar), a map is treated as structured inline authoring
// and marshaled to YAML — mirroring Kubernetes's `manifests:` string-or-map
// entries. `path` stays string-only, unchanged from the pre-rename `template`
// key's file-reference role. The two are mutually exclusive.
func resolveTemplateSection(componentSection map[string]any, stackName string) (templateBody, templatePath string, err error) {
	templatePath, _ = componentSection[cfg.TemplatePathSectionName].(string)

	switch v := componentSection[cfg.TemplateSectionName].(type) {
	case string:
		templateBody = v
	case map[string]any:
		if len(v) > 0 {
			marshaled, marshalErr := yaml.Marshal(v)
			if marshalErr != nil {
				return "", "", fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, marshalErr)
			}
			templateBody = string(marshaled)
		}
	}

	if templateBody != "" && templatePath != "" {
		return "", "", fmt.Errorf("%w: stack %q", errUtils.ErrAwsCloudFormationTemplateAndPathMutuallyExclusive, stackName)
	}
	return templateBody, templatePath, nil
}

// isAbstractComponent reports whether the component is marked `metadata.type: abstract`,
// exempting it from having a template (abstract components exist only to be inherited from).
func isAbstractComponent(componentSection map[string]any) bool {
	metadata, ok := componentSection["metadata"].(map[string]any)
	if !ok {
		return false
	}
	componentType, ok := metadata["type"].(string)
	return ok && componentType == "abstract"
}

// normalizeParameters converts the `parameters:` section to CloudFormation API
// parameters. Two shapes are accepted:
//
//   - a map of parameter name to value (the native Atmos form), and
//   - a list of `{ParameterKey, ParameterValue[, UsePreviousValue]}` entries,
//     the AWS CLI / Rain form, so existing parameter files (including a JSON
//     array pulled in with `!include`) keep working unchanged.
//
// Any other non-nil value is an error rather than silently deploying the
// template's defaults. Per the Parameter Typing contract: scalars are
// stringified, and list values are comma-joined to match CloudFormation's
// List<Type>/CommaDelimitedList wire format (the API accepts only strings).
func normalizeParameters(raw any) ([]cfntypes.Parameter, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return normalizeParameterMap(v)
	case []any:
		return normalizeParameterList(v)
	default:
		return nil, fmt.Errorf("%w: 'parameters' must be a map of name to value or a list of {ParameterKey, ParameterValue} entries, got %T", errUtils.ErrInvalidAwsCloudFormationParameters, raw)
	}
}

// normalizeParameterMap converts the native `parameters:` map form, sorted by
// name for deterministic output.
func normalizeParameterMap(params map[string]any) ([]cfntypes.Parameter, error) {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]cfntypes.Parameter, 0, len(keys))
	for _, key := range keys {
		value, err := stringifyParameterValue(params[key])
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", key, err)
		}
		result = append(result, cfntypes.Parameter{
			ParameterKey:   awsString(key),
			ParameterValue: awsString(value),
		})
	}
	return result, nil
}

// normalizeParameterList converts the AWS CLI / Rain list form, preserving the
// declared order. Malformed entries are errors that name the entry index.
func normalizeParameterList(entries []any) ([]cfntypes.Parameter, error) {
	result := make([]cfntypes.Parameter, 0, len(entries))
	seen := make(map[string]int, len(entries))
	for i, entry := range entries {
		param, err := normalizeParameterEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("parameters[%d]: %w", i, err)
		}
		key := *param.ParameterKey
		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf("parameters[%d]: %w: duplicate ParameterKey %q (first declared at parameters[%d])", i, errUtils.ErrInvalidAwsCloudFormationParameters, key, first)
		}
		seen[key] = i
		result = append(result, param)
	}
	return result, nil
}

// normalizeParameterEntry converts one `{ParameterKey, ParameterValue,
// UsePreviousValue}` list entry. UsePreviousValue keeps the value the stack
// already has, so it excludes a ParameterValue.
func normalizeParameterEntry(entry any) (cfntypes.Parameter, error) {
	fields, ok := entry.(map[string]any)
	if !ok {
		return cfntypes.Parameter{}, fmt.Errorf("%w: each entry must be a map with ParameterKey and ParameterValue, got %T", errUtils.ErrInvalidAwsCloudFormationParameters, entry)
	}
	name, usePrevious, err := parseParameterEntryFields(fields)
	if err != nil {
		return cfntypes.Parameter{}, err
	}
	if usePrevious {
		return cfntypes.Parameter{ParameterKey: awsString(name), UsePreviousValue: &usePrevious}, nil
	}

	value, err := stringifyParameterValue(fields[paramValueField])
	if err != nil {
		return cfntypes.Parameter{}, fmt.Errorf("parameter %q: %w", name, err)
	}
	return cfntypes.Parameter{ParameterKey: awsString(name), ParameterValue: awsString(value)}, nil
}

// parseParameterEntryFields validates one list entry's field names and types
// and returns its parameter name and UsePreviousValue flag.
func parseParameterEntryFields(fields map[string]any) (name string, usePrevious bool, err error) {
	for field := range fields {
		if field != paramKeyField && field != paramValueField && field != paramUsePreviousField {
			return "", false, fmt.Errorf("%w: unknown field %q (expected %s, %s, or %s)", errUtils.ErrInvalidAwsCloudFormationParameters, field, paramKeyField, paramValueField, paramUsePreviousField)
		}
	}

	name, _ = fields[paramKeyField].(string)
	if name == "" {
		return "", false, fmt.Errorf("%w: %s must be a non-empty string", errUtils.ErrInvalidAwsCloudFormationParameters, paramKeyField)
	}

	if rawUse, present := fields[paramUsePreviousField]; present {
		var ok bool
		if usePrevious, ok = rawUse.(bool); !ok {
			return "", false, fmt.Errorf("%w: %s for %q must be a boolean, got %T", errUtils.ErrInvalidAwsCloudFormationParameters, paramUsePreviousField, name, rawUse)
		}
	}
	if _, hasValue := fields[paramValueField]; usePrevious && hasValue {
		return "", false, fmt.Errorf("%w: %q sets both %s and %s", errUtils.ErrInvalidAwsCloudFormationParameters, name, paramUsePreviousField, paramValueField)
	}
	return name, usePrevious, nil
}

// stringifyParameterValue normalizes a single parameter value to CloudFormation's
// string wire format: scalars are stringified directly, lists are comma-joined
// (List<Type>/CommaDelimitedList).
func stringifyParameterValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			s, err := stringifyParameterValue(item)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ","), nil
	case nil:
		return "", nil
	case map[string]any:
		return "", fmt.Errorf("%w: parameter values must be scalars or lists, got a map", errUtils.ErrInvalidAwsCloudFormationSettings)
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// normalizeCapabilities converts `capabilities:` (a list of strings) to CloudFormation
// capability enum values, rejecting anything the SDK does not define so a typo
// fails locally with the valid set instead of as a late AWS API error.
func normalizeCapabilities(raw any) ([]cfntypes.Capability, error) {
	items := normalizeStringSlice(raw)
	if len(items) == 0 {
		return nil, nil
	}
	valid := cfntypes.Capability("").Values()
	result := make([]cfntypes.Capability, 0, len(items))
	for _, item := range items {
		capability := cfntypes.Capability(item)
		if !slices.Contains(valid, capability) {
			return nil, fmt.Errorf("%w: %q is not a valid capability (valid: %s)", errUtils.ErrInvalidAwsCloudFormationCapabilities, item, joinCapabilities(valid))
		}
		result = append(result, capability)
	}
	return result, nil
}

// joinCapabilities renders capability enum values as a comma-separated list.
func joinCapabilities(values []cfntypes.Capability) string {
	names := make([]string, 0, len(values))
	for _, v := range values {
		names = append(names, string(v))
	}
	return strings.Join(names, ", ")
}

// normalizeTags converts `tags:` (a map[string]string-ish) to CloudFormation tags,
// sorted by key for deterministic output.
func normalizeTags(raw any) []cfntypes.Tag {
	tags, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]cfntypes.Tag, 0, len(keys))
	for _, key := range keys {
		value := fmt.Sprintf("%v", tags[key])
		result = append(result, cfntypes.Tag{
			Key:   awsString(key),
			Value: awsString(value),
		})
	}
	return result
}

// normalizeStringSlice converts a YAML-decoded []any (or already-[]string) into a
// []string, skipping non-string entries.
func normalizeStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// toInt32 converts a YAML-decoded numeric value (commonly int, but YAML decoders
// may produce other numeric kinds) to int32, defaulting to 0 for unrecognized types.
func toInt32(raw any) int32 {
	switch v := raw.(type) {
	case int:
		return clampToInt32(int64(v))
	case int32:
		return v
	case int64:
		return clampToInt32(v)
	case float64:
		return clampToInt32(int64(v))
	default:
		return 0
	}
}

// clampToInt32 saturates an int64 to the int32 range rather than silently
// wrapping, since timeout_in_minutes is a small positive number in practice
// and a config typo should clamp visibly rather than overflow.
func clampToInt32(v int64) int32 {
	switch {
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	default:
		return int32(v)
	}
}

// awsString is a local alias for aws.String, avoiding an extra import at every call site.
func awsString(s string) *string {
	return &s
}
