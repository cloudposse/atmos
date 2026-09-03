package cloudformation

import (
	"fmt"
	"math"
	"sort"
	"strings"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// stackSpec is the fully-resolved, SDK-ready shape of an aws/cloudformation component,
// extracted from the component's stack config section.
type stackSpec struct {
	StackName             string
	TemplatePath          string
	TemplateAbsPath       string
	TemplateBody          string
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
	if templateBody == "" && templatePath == "" && !isAbstractComponent(componentSection) {
		return nil, errUtils.ErrMissingAwsCloudFormationTemplate
	}

	spec := &stackSpec{
		StackName:    stackName,
		TemplatePath: templatePath,
		TemplateBody: templateBody,
	}

	params, err := normalizeParameters(componentSection[cfg.ParametersSectionName])
	if err != nil {
		return nil, err
	}
	spec.Parameters = params

	spec.Capabilities = normalizeCapabilities(componentSection[cfg.CapabilitiesSectionName])

	spec.Tags = normalizeTags(componentSection[cfg.TagsSectionName])

	if stackPolicy, ok := componentSection[cfg.StackPolicySectionName].(map[string]any); ok {
		spec.StackPolicyFile, _ = stackPolicy["file"].(string)
	}

	spec.RoleArn, _ = componentSection[cfg.RoleArnSectionName].(string)
	spec.NotificationArns = normalizeStringSlice(componentSection[cfg.NotificationArnsSectionName])
	spec.DisableRollback, _ = componentSection[cfg.DisableRollbackSectionName].(bool)
	spec.TerminationProtection, _ = componentSection[cfg.TerminationProtectionSectionName].(bool)

	if timeout, ok := componentSection[cfg.TimeoutInMinutesSectionName]; ok {
		spec.TimeoutInMinutes = toInt32(timeout)
	}

	return spec, nil
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

// normalizeParameters converts the `parameters:` map to CloudFormation API parameters.
// Per the Parameter Typing contract: scalars are stringified, and list values are
// comma-joined to match CloudFormation's List<Type>/CommaDelimitedList wire format
// (the API accepts only strings). UsePreviousValue is not expressible in config —
// Atmos config is always the source of truth.
func normalizeParameters(raw any) ([]cfntypes.Parameter, error) {
	params, ok := raw.(map[string]any)
	if !ok {
		return nil, nil
	}

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
// capability enum values.
func normalizeCapabilities(raw any) []cfntypes.Capability {
	items := normalizeStringSlice(raw)
	if len(items) == 0 {
		return nil
	}
	result := make([]cfntypes.Capability, 0, len(items))
	for _, item := range items {
		result = append(result, cfntypes.Capability(item))
	}
	return result
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
