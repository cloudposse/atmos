package plandiff

import (
	"fmt"
	"slices"

	"github.com/cloudposse/atmos/pkg/perf"
)

// knownAfterApplyText is displayed for attribute values Terraform cannot determine until apply.
const knownAfterApplyText = "(known after apply)"

type options struct {
	ignoreChanges []string
	legacyDisplay bool
	// origUnknown and newUnknown hold top-level attributes reported in change.after_unknown
	// for the original and new plans. They affect display only, never the diff verdict.
	origUnknown map[string]bool
	newUnknown  map[string]bool
}

// Option customizes how two plans are compared.
type Option func(*options)

// newOptions applies the given options to a zero-value options struct.
func newOptions(opts ...Option) options {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithIgnoreChanges adds validated resource_type.attribute exclusions. Excluded attributes are
// removed from both the verdict and the display.
func WithIgnoreChanges(entries []string) Option {
	defer perf.Track(nil, "plandiff.WithIgnoreChanges")()

	return func(o *options) {
		o.ignoreChanges = append(o.ignoreChanges, entries...)
	}
}

// WithLegacyDisplay preserves the formatter-only attribute hiding of older editions.
// Hidden attributes still participate in the comparison unless they are explicitly excluded.
func WithLegacyDisplay(legacy bool) Option {
	defer perf.Track(nil, "plandiff.WithLegacyDisplay")()

	return func(o *options) {
		o.legacyDisplay = legacy
	}
}

// withUnknownAttributes records the attributes that are unknown in each plan's resource.
func withUnknownAttributes(origUnknown, newUnknown map[string]bool) Option {
	return func(o *options) {
		o.origUnknown = origUnknown
		o.newUnknown = newUnknown
	}
}

// removedAttributeLine renders an attribute that is missing from the new plan. When the new
// plan reports it as unknown, the value is becoming known after apply rather than being removed.
func (o options) removedAttributeLine(attribute string, origValue interface{}) string {
	if o.newUnknown[attribute] {
		return fmt.Sprintf("  ~ %s: %v => %s\n", attribute, formatValue(origValue), knownAfterApplyText)
	}
	return fmt.Sprintf("  - %s: %v\n", attribute, formatValue(origValue))
}

// addedAttributeLine renders an attribute that is missing from the original plan. When the
// original plan reports it as unknown, the value was unknown and is now known.
func (o options) addedAttributeLine(attribute string, newValue interface{}) string {
	if o.origUnknown[attribute] {
		return fmt.Sprintf("  ~ %s: %s => %v\n", attribute, knownAfterApplyText, formatValue(newValue))
	}
	return fmt.Sprintf("  + %s: %v\n", attribute, formatValue(newValue))
}

// ignores matches configured exclusions exactly, without automatic attribute exclusions.
func (o options) ignores(resourceType, attribute string) bool {
	return slices.Contains(o.ignoreChanges, resourceType+"."+attribute)
}

// legacyHiddenAttribute preserves the formatter-only exclusions for older editions.
// These attributes still participate in comparison unless explicitly excluded.
func legacyHiddenAttribute(attribute string) bool {
	switch attribute {
	case "response_body_base64", "content_base64sha256", "content_base64sha512",
		"content_md5", "content_sha1", "content_sha256", "content_sha512":
		return true
	default:
		return false
	}
}
