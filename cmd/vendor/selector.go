package vendor

import (
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	pkgtags "github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// vendorSelectorGroupCount reports how many of the three mutually exclusive vendor selector
// concepts are in use: --component (a single explicit name), --tags (vendor.yaml's own declared
// source tags), and --stack/--labels (the stack-resolved component set -- --stack and --labels
// compose with each other, so together they count as one group). Shared by `vendor diff`, `vendor
// clean`, and `vendor verify` to validate their selector flags before resolving components.
func vendorSelectorGroupCount(component string, filterTags []string, stack string, labels map[string]string) int {
	count := 0
	for _, present := range []bool{component != "", len(filterTags) > 0, stack != "" || len(labels) > 0} {
		if present {
			count++
		}
	}
	return count
}

// errVendorSelectorsExclusive builds the shared "pick exactly one selector" error for
// --component/--tags/--stack/--labels, worded identically across `vendor diff`, `vendor clean`, and
// `vendor verify`.
func errVendorSelectorsExclusive() error {
	return errUtils.Build(errUtils.ErrInvalidArgumentError).
		WithExplanation("--component, --tags, and --stack/--labels are mutually exclusive selectors.").
		WithHint("Pass at most one: --component for a single target, --tags for vendor.yaml source tags, or --stack/--labels for a stack-resolved component set.").
		Err()
}

// VendorSelectorOptions bundles the --component/--tags/--stack/--labels selector flags shared by
// `vendor diff`, `vendor clean`, and `vendor verify` (an Options-pattern struct: individually these
// are simple values, but resolveVendorSelectorComponents' signature grew past a readable positional
// parameter count once --stack/--labels joined --component/--tags).
type VendorSelectorOptions struct {
	AtmosConfig   *schema.AtmosConfiguration
	Component     string
	Tags          []string
	Stack         string
	Labels        map[string]string
	VendorFile    string
	ComponentType string
	TypeChanged   bool
}

// resolveVendorSelectorComponents resolves --component/--tags/--stack/--labels (already validated
// mutually exclusive by the caller) into a component-name list. A nil result with no error means no
// selector was given at all -- callers decide what that means for them (e.g. `vendor clean`/`vendor
// verify` treat it as "operate on everything"; `vendor diff` rejects it as "selector required").
//
// When a selector WAS given (--tags or --stack/--labels) but resolves to zero components (e.g. an
// unknown stack name, or tags/labels matching nothing), this returns an error rather than nil --
// silently falling through to nil here would be indistinguishable from "no selector given" to every
// caller, making `vendor clean --stack typo-d-stack-name` clean EVERYTHING instead of reporting no
// match. --component can't hit this path: an explicit single name always resolves to exactly itself.
func resolveVendorSelectorComponents(opts *VendorSelectorOptions) ([]string, error) {
	var components []string
	var err error

	switch {
	case opts.Component != "":
		return []string{opts.Component}, nil
	case len(opts.Tags) > 0:
		components, err = resolveVendorTagsSelector(opts.VendorFile, opts.Tags)
	case opts.Stack != "" || len(opts.Labels) > 0:
		components, err = resolveVendorStackLabelsSelector(opts.AtmosConfig, opts.Stack, opts.Labels, opts.ComponentType, opts.TypeChanged)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(components) == 0 {
		return nil, errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation("No components matched the given selector.").
			Err()
	}
	return components, nil
}

// resolveVendorTagsSelector resolves --tags (vendor.yaml's own declared source tags, matches any)
// into a deduped, sorted list of component names, reading the same manifest --component/--file
// resolution already uses (vendoring.ListDeclaredSources: --file override, else ./vendor.yaml,
// following imports -- NOT pkg/list's directory-scanning GetVendorInfos, which requires
// vendor.base_path to be configured in atmos.yaml; `vendor pull --tags`/`vendor update --tags` have
// never required that, and this selector must not impose a new, inconsistent requirement). Shared
// by `vendor diff`, `vendor clean`, and `vendor verify`. A missing vendor.yaml (no manifest at all)
// returns an empty result, not an error -- exactly like `vendor pull --tags` finds nothing to filter
// in a component.yaml-only repo.
func resolveVendorTagsSelector(vendorFile string, filterTags []string) ([]string, error) {
	sources, ok, err := vendoring.ListDeclaredSources(vendorFile)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	seen := make(map[string]bool)
	var names []string
	for i := range sources {
		source := &sources[i]
		if !pkgtags.MatchesTags(source.Tags, filterTags, pkgtags.TagModeAny) {
			continue
		}
		if seen[source.Component] {
			continue
		}
		seen[source.Component] = true
		names = append(names, source.Component)
	}

	sort.Strings(names)
	return names, nil
}

// resolveVendorStackLabelsSelector resolves --stack/--labels (both filter the stack-resolved
// component set -- a distinct, component-metadata concept from --tags' vendor-manifest source
// tags) into a deduped, sorted list of component names via
// internal/exec.ResolveVendorComponentSelector. Shared by `vendor update`, `vendor diff`, `vendor
// clean`, and `vendor verify`.
func resolveVendorStackLabelsSelector(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	labels map[string]string,
	componentType string,
	typeChanged bool,
) ([]string, error) {
	componentTypes := []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType}
	if typeChanged {
		componentTypes = []string{componentType}
	}
	return e.ResolveVendorComponentSelector(atmosConfig, stack, labels, componentTypes)
}
