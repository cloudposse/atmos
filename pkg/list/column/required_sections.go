package column

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
	atmostemplate "github.com/cloudposse/atmos/pkg/template"
)

// sectionBackedFields maps a column-template top-level field name to the identically named
// top-level Atmos component section it is a verbatim passthrough of. Every list command's row
// extraction (pkg/list/extract) copies these sections onto the row map under the same key it
// uses in the raw ComponentSection (e.g. extract/stacks.go's `stack["vars"] = vars`,
// extract/metadata.go's `"settings": instance.Settings`), so a column referencing one of these
// fields requires that section to be fully evaluated (Go templates + YAML functions) rather than
// left as raw/merged text.
var sectionBackedFields = map[string]string{
	"vars":     "vars",
	"settings": "settings",
	"metadata": "metadata",
	"env":      "env",
	"backend":  "backend",
}

// derivedFields lists top-level column-template fields that pkg/list/extract computes from
// already-cheap identifiers (stack/component names) or from `metadata` (enabled, locked, tags,
// labels, status, type, ...). Referencing one of these does not, by itself, require any
// additional section: callers that need `metadata` for filtering/status derivation (list
// components, list instances) fold it into the required set independently of column selection —
// see cmd/list/components.go and pkg/list/list_instances.go.
var derivedFields = map[string]struct{}{
	"stack": {}, "component": {}, "atmos_component": {}, "atmos_stack": {},
	"atmos_component_type": {}, "enabled": {}, "locked": {}, "abstract": {},
	"file": {}, "name": {}, "workflow": {}, "description": {}, "steps": {},
	"components": {}, "atmos_vendor_type": {}, "atmos_vendor_file": {}, "atmos_vendor_target": {},
	"status": {}, "status_text": {}, "component_type": {}, "component_folder": {},
	"component_base": {}, "inherits": {}, "type": {}, "tags": {}, "labels": {}, "stack_count": {},
}

// RequiredSections statically determines which top-level Atmos component sections (vars,
// settings, metadata, env, backend) the given column templates actually reference, by walking
// each column's parsed Go-template AST for field references (see pkg/template.ExtractFieldRefs)
// whose first path segment names a section.
//
// Returns ok=false whenever any column can't be statically resolved this way: a catch-all `.raw`
// reference (which exposes the entire row, including every section), a `range`/`with` block
// (which rebinds "." — see pkg/template.HasDynamicScope), an unparseable template, or any
// top-level field this function does not recognize as either section-backed or derived. Callers
// MUST treat ok=false as "pass nil / evaluate everything": under-computing the required set is
// far worse than over-computing it, since a displayed column would then show raw, unevaluated
// text instead of falling back cleanly to full evaluation.
//
// A true result with an empty (non-nil) `sections` slice is a real, meaningful answer — "no
// section is required" — and is distinct from a nil slice. Callers MUST preserve that
// distinction (e.g. by passing the returned slice through unchanged, never coalescing an empty
// result to nil) since downstream evaluation-gating treats nil as "no filter" (see
// internal/exec's isSectionRequired).
func RequiredSections(columns []Config) (sections []string, ok bool) {
	defer perf.Track(nil, "list.column.RequiredSections")()

	required := make(map[string]struct{})

	for _, col := range columns {
		dynamic, err := atmostemplate.HasDynamicScope(col.Value)
		if err != nil || dynamic {
			return nil, false
		}

		refs, err := atmostemplate.ExtractFieldRefs(col.Value)
		if err != nil {
			return nil, false
		}

		if !collectRequiredSections(refs, required) {
			return nil, false
		}
	}

	result := make([]string, 0, len(required))
	for s := range required {
		result = append(result, s)
	}
	sort.Strings(result)

	return result, true
}

// EnsureSection returns sections with name added if not already present, preserving a non-nil
// result: an empty (but non-nil) input slice stays non-nil, matching isSectionRequired's contract
// that a non-nil filter -- even an empty one -- means "gating is active," never "no filter".
//
// Callers use this to fold in a section their row-extraction pipeline always needs regardless of
// column selection (e.g. `metadata`, which list components/instances always read for
// enabled/locked/tags/labels/status derivation -- see cmd/list/components.go and
// pkg/list/list_instances.go) into the result of RequiredSections.
func EnsureSection(sections []string, name string) []string {
	for _, s := range sections {
		if s == name {
			return sections
		}
	}
	return append(sections, name)
}

// collectRequiredSections folds the section references from refs into required, returning false
// the moment it encounters a reference RequiredSections cannot safely resolve.
func collectRequiredSections(refs []atmostemplate.FieldRef, required map[string]struct{}) bool {
	for _, ref := range refs {
		if len(ref.Path) == 0 {
			continue
		}
		top := ref.Path[0]

		if top == "raw" {
			return false
		}
		if section, known := sectionBackedFields[top]; known {
			required[section] = struct{}{}
			continue
		}
		if _, known := derivedFields[top]; known {
			continue
		}
		// Unrecognized top-level field: cannot prove it is section-free. Fall back to full eval.
		return false
	}
	return true
}
