package auth

import (
	"encoding/json"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	authpkg "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// authValidationScope keeps each component's auth independent: a sibling's
// principal must never complete an otherwise invalid identity in this scope.
type authValidationScope struct {
	stack     string
	component string
	kind      string
	section   map[string]any
}

// validateStackAuth uses the normal stack inheritance and auth merge paths, but
// disables evaluation at both manifest-load and component-processing stages.
func validateStackAuth(atmosConfig *schema.AtmosConfiguration, stack, component string) error {
	staticConfig := *atmosConfig
	staticConfig.Templates.Settings.Enabled = false
	staticConfig.AuthManager = nil
	var components []string
	if component != "" {
		components = []string{component}
	}
	stacks, err := e.ExecuteDescribeStacksWithOptions(
		&staticConfig, stack, components, nil, nil,
		false, false, false, false, nil, nil, true, e.DescribeStacksErrorOptions{StaticOnly: true},
	)
	if err != nil {
		return err
	}
	scopes := collectAuthValidationScopes(stacks)
	if len(scopes) == 0 {
		return fmt.Errorf("%w: no concrete components match stack %q and component %q", errUtils.ErrInvalidAuthConfig, stack, component)
	}
	for _, scope := range scopes {
		if err := validateComponentAuth(&staticConfig, scope.section); err != nil {
			return fmt.Errorf("stack %q, component %q (%s): %w", scope.stack, scope.component, scope.kind, err)
		}
	}
	return nil
}

func collectAuthValidationScopes(stacks map[string]any) []authValidationScope {
	var scopes []authValidationScope
	for stack, rawStack := range stacks {
		stackSection, _ := rawStack.(map[string]any)
		components, _ := stackSection[cfg.ComponentsSectionName].(map[string]any)
		for kind, rawComponents := range components {
			typedComponents, _ := rawComponents.(map[string]any)
			for component, rawSection := range typedComponents {
				section, ok := rawSection.(map[string]any)
				if !ok {
					continue
				}
				metadata, _ := section[cfg.MetadataSectionName].(map[string]any)
				if metadata["type"] == "abstract" {
					continue
				}
				scopes = append(scopes, authValidationScope{stack: stack, component: component, kind: kind, section: section})
			}
		}
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].stack != scopes[j].stack {
			return scopes[i].stack < scopes[j].stack
		}
		if scopes[i].component != scopes[j].component {
			return scopes[i].component < scopes[j].component
		}
		return scopes[i].kind < scopes[j].kind
	})
	return scopes
}

func validateComponentAuth(atmosConfig *schema.AtmosConfiguration, section map[string]any) error {
	templateDelimiters, err := delimiters.Parse(atmosConfig.Templates.Settings.Delimiters)
	if err != nil {
		return err
	}
	explicitTags, err := u.ExplicitYAMLTags(atmosConfig)
	if err != nil {
		return err
	}
	// Inspect before decoding as a dynamic expression may stand in for an entire
	// mapping (e.g. principal: !template ...), which cannot decode to AuthConfig.
	if err := rejectDynamicAuthValues(section[cfg.AuthSectionName], cfg.AuthSectionName, templateDelimiters, !explicitTags); err != nil {
		return err
	}
	if raw, present := section[cfg.AuthSectionName]; present && raw != nil {
		if _, ok := raw.(map[string]any); !ok {
			return fmt.Errorf("%w: component auth must be a mapping", errUtils.ErrInvalidAuthConfig)
		}
	}
	merged, err := authpkg.MergeComponentAuthFromConfig(&atmosConfig.Auth, section, atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return err
	}
	if err := rejectMergedDynamicAuthValues(merged, templateDelimiters, !explicitTags); err != nil {
		return err
	}
	return validation.NewValidator().ValidateAuthConfig(merged)
}

func rejectMergedDynamicAuthValues(authConfig *schema.AuthConfig, templateDelimiters delimiters.Delimiters, legacyFunctions bool) error {
	// mapstructure retains typed provider/identity maps inside AuthConfigToMap.
	// Normalize them recursively while preserving internal expression strings;
	// public YAML conversion would restore tags and lose that identity here.
	encoded, err := json.Marshal(authConfig)
	if err != nil {
		return fmt.Errorf("%w: auth configuration: %w", errUtils.ErrEncode, err)
	}
	var values map[string]any
	if err := json.Unmarshal(encoded, &values); err != nil {
		return fmt.Errorf("%w: auth configuration: %w", errUtils.ErrDecode, err)
	}
	return rejectDynamicAuthValues(values, cfg.AuthSectionName, templateDelimiters, legacyFunctions)
}

// rejectDynamicAuthValues prevents a nonempty expression string from passing a
// provider's or identity's required-value check without ever being evaluated.
// Only paths are reported: auth values may contain credentials.
func rejectDynamicAuthValues(value any, path string, templateDelimiters delimiters.Delimiters, legacyFunctions bool) error {
	switch typed := value.(type) {
	case string:
		if isDynamicAuthString(typed, templateDelimiters, legacyFunctions) {
			return fmt.Errorf("%w: cannot fully validate %s: unresolved dynamic auth value; scoped auth validation does not evaluate templates or YAML functions", errUtils.ErrInvalidAuthConfig, path)
		}
	case map[string]any:
		return rejectDynamicAuthMapValues(typed, path, templateDelimiters, legacyFunctions)
	case []any:
		for index, item := range typed {
			if err := rejectDynamicAuthValues(item, fmt.Sprintf("%s[%d]", path, index), templateDelimiters, legacyFunctions); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectDynamicAuthMapValues(values map[string]any, path string, templateDelimiters delimiters.Delimiters, legacyFunctions bool) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := rejectDynamicAuthValues(values[key], path+"."+key, templateDelimiters, legacyFunctions); err != nil {
			return err
		}
	}
	return nil
}

// Native tags retain their identity through decoding and auth merges. Untagged
// function-like text is executable only in editions before explicit YAML tags.
func isDynamicAuthString(value string, templateDelimiters delimiters.Delimiters, legacyFunctions bool) bool {
	_, native := yamlsource.Decode(value)
	return native || starlarksource.IsEncoded(value) || templateDelimiters.ContainsAny(value) ||
		(legacyFunctions && (fntag.MatchesStringForm(value) ||
			fntag.MatchesTag(value, fntag.ToYAML(fntag.Include)) || fntag.MatchesTag(value, fntag.ToYAML(fntag.IncludeRaw))))
}
