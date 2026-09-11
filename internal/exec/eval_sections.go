package exec

import "slices"

// isSectionRequired reports whether sectionName must be evaluated (rendered as a Go template
// and/or have its YAML functions resolved) given an eval-sections filter.
//
// A nil filter disables gating entirely: every section is required, matching the historical
// eager-evaluation behavior exactly. This is what every existing caller gets (describe stacks,
// terraform, helmfile, hooks, etc.) since only the `list stacks`/`list components`/
// `list instances` commands ever construct a non-nil filter (see column.RequiredSections). A
// non-nil filter (even an empty one, meaning "no column needs any section") gates by membership.
//
// Deliberately checks `sections == nil`, NOT `len(sections) == 0`: an empty-but-non-nil slice is
// a real, valid "nothing is required" filter and must NOT be treated the same as "no filter" --
// collapsing the two (as a bare len() check would) would silently disable gating exactly when it
// matters most (default `list stacks` columns, which reference no section at all). Do not
// "simplify" this to `len(sections) == 0` -- that reintroduces the spurious eager-evaluation bug
// this file exists to fix.
func isSectionRequired(sections []string, sectionName string) bool {
	if sections == nil {
		return true
	}
	return slices.Contains(sections, sectionName)
}

// splitSectionsByRequirement returns a shallow copy of componentSection restricted to the
// top-level sections isSectionRequired approves, plus the sections that were left out so the
// caller can restore them untouched afterward (see restoreNonTemplatedSections, reused here
// generically since "copy excluded keys back in" is identical for both use cases).
//
// A nil filter (gating disabled) returns the input unchanged with a nil excluded map -- the same
// "nothing was split" shape splitNonTemplatedSections uses for its own no-op case, so both
// exclusion sources can be restored through the same helper without special-casing nil.
func splitSectionsByRequirement(componentSection map[string]any, sections []string) (filtered, excluded map[string]any) {
	if sections == nil {
		return componentSection, nil
	}

	filtered = make(map[string]any, len(componentSection))
	excluded = make(map[string]any, len(componentSection))
	for key, value := range componentSection {
		if isSectionRequired(sections, key) {
			filtered[key] = value
		} else {
			excluded[key] = value
		}
	}
	return filtered, excluded
}
