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
		return nil, err
	}
	var changes []managers.FileChange
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
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
		return jsonOptions{}, fmt.Errorf("%w: %w", errUtils.ErrVersionJSONOptionsInvalid, err)
	}
	return opts, nil
}

// applySets writes every configured set entry into content, skipping entries
// whose dependency is not locked (same skip-silently idiom as the marker and
// github-actions managers) and entries whose value already matches (avoiding
// an unnecessary sjson re-render and strengthening idempotency).
func applySets(content []byte, entries []setEntry, refs map[string]manager.VersionRef) ([]byte, error) {
	current := content
	for _, entry := range entries {
		ref, ok := refs[entry.From]
		if !ok || ref.Version == "" {
			continue
		}
		value := ref.String()
		if gjson.GetBytes(current, entry.Path).String() == value {
			continue
		}
		updated, err := sjson.SetBytes(current, entry.Path, value)
		if err != nil {
			return nil, fmt.Errorf("%w: path %q: %w", errUtils.ErrVersionJSONSetFailed, entry.Path, err)
		}
		current = updated
	}
	return current, nil
}

func init() {
	managers.Register(Manager{})
}
