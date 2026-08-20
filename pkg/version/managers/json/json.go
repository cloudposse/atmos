// Package json implements the json file manager: sjson/gjson-based in-place
// field writes on plain JSON files (package manifests, plugin listings, and
// similar), configured via `options.set: [{path, from}]`. Unlike
// marker (comment-annotated) or template (a *.tmpl source rendered to a
// sibling file), sjson.Set patches only the targeted path and leaves the
// rest of the document's bytes -- formatting, key order, whitespace --
// untouched, so this file must never import "encoding/json" alongside the
// local "json" package name.
package json

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

// Name is the manager's registry name.
const Name = "json"

// complexPathChars are sjson/gjson path characters that select more than one
// location (wildcards, queries) rather than a single simple field.
const complexPathChars = "#*?@"

// appendPathSegment is sjson's array-append marker.
const appendPathSegment = "-1"

// setEntry is one options.set rule: write the resolved value for the
// dependency named From at the sjson/gjson dot-path Path.
type setEntry struct {
	Path string `mapstructure:"path"`
	From string `mapstructure:"from"`
}

// jsonOptions is the parsed shape of a json file rule's Options.
type jsonOptions struct {
	Set []setEntry `mapstructure:"set"`
}

// Manager writes locked values into JSON files at configured field paths.
type Manager struct{}

// Name returns the manager's registry name.
func (Manager) Name() string {
	defer perf.Track(nil, "json.Manager.Name")()

	return Name
}

// DefaultPaths is empty: unlike *.tmpl files or the fixed workflows
// directory, there is no generically-safe default glob for "JSON files this
// tool owns" -- the json manager only runs over configured paths.
func (Manager) DefaultPaths() []string {
	defer perf.Track(nil, "json.Manager.DefaultPaths")()

	return nil
}

// Plan scans the configured files and returns the field writes needed to
// match the locked versions.
func (Manager) Plan(ctx context.Context, in *managers.Input) ([]managers.FileChange, error) {
	defer perf.Track(in.Config, "json.Manager.Plan")()

	if len(in.Paths) == 0 {
		return nil, nil
	}
	opts, err := parseOptions(in.Options)
	if err != nil {
		return nil, err
	}
	if len(opts.Set) == 0 {
		return nil, nil
	}
	files, err := managers.ExpandPaths(in.Dir, in.Paths)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrVersionJSONExpandPathsFailed, in.Dir, err)
	}
	var changes []managers.FileChange
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrVersionJSONReadFailed, file, err)
		}
		if !gjson.ValidBytes(content) {
			return nil, fmt.Errorf("%w: %s", errUtils.ErrVersionJSONInvalidContent, file)
		}
		updated, err := applySets(content, opts.Set, in.Refs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if !bytes.Equal(content, updated) {
			changes = append(changes, managers.FileChange{Path: file, Old: content, New: updated})
		}
	}
	return changes, nil
}

// parseOptions decodes a file rule's Options into the json manager's typed
// shape.
func parseOptions(raw map[string]any) (jsonOptions, error) {
	var opts jsonOptions
	if raw == nil {
		return opts, nil
	}
	if err := mapstructure.Decode(raw, &opts); err != nil {
		return jsonOptions{}, errUtils.Build(errUtils.ErrVersionJSONOptionsInvalid).
			WithCause(err).
			WithHint(`Quote numeric path segments, e.g. path: "0", so YAML doesn't parse them as an integer`).
			Err()
	}
	if dup := duplicatePath(opts.Set); dup != "" {
		return jsonOptions{}, fmt.Errorf("%w: %q", errUtils.ErrVersionJSONDuplicatePath, dup)
	}
	return opts, nil
}

// duplicatePath returns the first path targeted by more than one set entry,
// or "" if every path is unique. Two entries targeting the same path would
// otherwise silently last-win with no indication the first write was
// discarded.
func duplicatePath(entries []setEntry) string {
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if seen[entry.Path] {
			return entry.Path
		}
		seen[entry.Path] = true
	}
	return ""
}

// isAppendPath reports whether path targets sjson's array-append marker (a
// "-1" segment anywhere in the path -- standalone, leading, trailing, or
// mid-path, e.g. items.-1.version). Append writes can never be applied
// idempotently -- there is no way to tell "already applied" from "not yet
// applied" by reading the target array -- so they are rejected outright
// rather than silently growing the array on every apply.
func isAppendPath(path string) bool {
	for _, segment := range strings.Split(path, ".") {
		if segment == appendPathSegment {
			return true
		}
	}
	return false
}

// isComplexPath reports whether path uses sjson/gjson's wildcard or query
// syntax, which can select zero, one, or many locations depending on the
// document's current content -- unlike a simple dotted path, which always
// names exactly one location and may be safely created when absent.
func isComplexPath(path string) bool {
	return strings.ContainsAny(path, complexPathChars)
}

// applySets writes every configured set entry into content, skipping entries
// whose dependency is not locked (same skip-silently idiom as the marker and
// github-actions managers).
func applySets(content []byte, entries []setEntry, refs map[string]manager.VersionRef) ([]byte, error) {
	current := content
	for _, entry := range entries {
		ref, ok := refs[entry.From]
		if !ok || ref.Version == "" {
			continue
		}
		updated, err := applySet(current, entry, ref.String())
		if err != nil {
			return nil, err
		}
		current = updated
	}
	return current, nil
}

// applySet writes one set entry's value into content, or returns content
// unchanged when the value already matches (avoiding an unnecessary sjson
// re-render and strengthening idempotency). It rejects an array-append path,
// a path whose current value is a container, and a complex/wildcard path
// that matches nothing -- see checkSetEntry for why each would otherwise be
// silently unsafe.
func applySet(content []byte, entry setEntry, value string) ([]byte, error) {
	if isAppendPath(entry.Path) {
		return nil, fmt.Errorf("%w: path %q", errUtils.ErrVersionJSONAppendPathUnsupported, entry.Path)
	}
	existing := gjson.GetBytes(content, entry.Path)
	if err := checkSetEntry(entry, &existing, value); err != nil {
		return nil, err
	}
	if !isComplexPath(entry.Path) && existing.Type == gjson.String && existing.String() == value {
		return content, nil
	}
	updated, err := sjson.SetBytes(content, entry.Path, value)
	if err != nil {
		return nil, fmt.Errorf("%w: path %q: %w", errUtils.ErrVersionJSONSetFailed, entry.Path, err)
	}
	return updated, nil
}

// checkSetEntry validates a set entry against the document's current state.
//
// A complex/wildcard path's gjson result is a synthesized array of every
// match, not a real container in the document, so it's checked only for
// whether it matched anything at all; a simple path's result is the actual
// current value, so it's checked for whether writing a scalar there would
// silently clobber an existing object or array.
func checkSetEntry(entry setEntry, existing *gjson.Result, value string) error {
	if isComplexPath(entry.Path) {
		if complexPathUnmatched(existing) {
			return fmt.Errorf("%w: path %q", errUtils.ErrVersionJSONPathNotFound, entry.Path)
		}
		return nil
	}
	targetsContainer := existing.Exists() && existing.String() != value && (existing.IsObject() || existing.IsArray())
	if targetsContainer {
		return fmt.Errorf("%w: path %q", errUtils.ErrVersionJSONPathTypeMismatch, entry.Path)
	}
	return nil
}

// complexPathUnmatched reports whether a complex/wildcard path's gjson
// result represents "no matches": either the path doesn't resolve at all, or
// it resolves to a zero-length synthesized array (an empty source array). A
// missing parent reports !Exists(); an empty source array reports Exists()
// with a zero-length synthesized array -- both mean "no matches".
func complexPathUnmatched(existing *gjson.Result) bool {
	return !existing.Exists() || (existing.IsArray() && len(existing.Array()) == 0)
}

func init() {
	managers.Register(Manager{})
}
