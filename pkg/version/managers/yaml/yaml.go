// Package yaml implements the yaml file manager: in-place field writes on
// existing YAML files (values files, extra config, and similar), configured
// via `options.set: [{path, from, format}]`. It reuses Atmos's own
// format-preserving YAML editor (pkg/yaml, the same engine behind `atmos
// config set` and `atmos stack set`), which edits via a yq expression rather
// than a full unmarshal/remarshal, so comments, anchors, and key order on
// untouched fields survive the write. Path syntax is the same dot-notation
// used by those commands (e.g. `sources[0].version`, `metadata."weird.key"`),
// not the sjson/gjson dialect the json manager uses.
package yaml

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// Name is the manager's registry name.
const Name = "yaml"

// setEntry is one options.set rule: write the resolved value for the
// dependency named From at the dot-notation Path. Format, when set, is a Go
// template (Sprig + Atmos template functions) rendered against the resolved
// manager.VersionRef whose output replaces the verbatim value -- e.g. a
// github-releases tag like "v1.228.0" reshaped to bare semver for a target
// that doesn't use the "v" convention.
type setEntry struct {
	Path   string `mapstructure:"path"`
	From   string `mapstructure:"from"`
	Format string `mapstructure:"format"`
}

// yamlOptions is the parsed shape of a yaml file rule's Options.
type yamlOptions struct {
	Set []setEntry `mapstructure:"set"`
}

// Manager writes locked values into YAML files at configured field paths.
type Manager struct{}

// Name returns the manager's registry name.
func (Manager) Name() string {
	defer perf.Track(nil, "yaml.Manager.Name")()

	return Name
}

// DefaultPaths is empty: like the json manager, there is no generically-safe
// default glob for "YAML files this tool owns" -- the yaml manager only runs
// over configured paths.
func (Manager) DefaultPaths() []string {
	defer perf.Track(nil, "yaml.Manager.DefaultPaths")()

	return nil
}

// Plan scans the configured files and returns the field writes needed to
// match the locked versions.
func (Manager) Plan(ctx context.Context, in *managers.Input) ([]managers.FileChange, error) {
	defer perf.Track(in.Config, "yaml.Manager.Plan")()

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
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrVersionYAMLExpandPathsFailed, in.Dir, err)
	}
	var changes []managers.FileChange
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrVersionYAMLReadFailed, file, err)
		}
		updated, err := applySets(content, opts.Set, in.Refs, managers.TemplateDelimiters(in.Config))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if !bytes.Equal(content, updated) {
			changes = append(changes, managers.FileChange{Path: file, Old: content, New: updated})
		}
	}
	return changes, nil
}

// parseOptions decodes a file rule's Options into the yaml manager's typed
// shape.
func parseOptions(raw map[string]any) (yamlOptions, error) {
	var opts yamlOptions
	if raw == nil {
		return opts, nil
	}
	if err := mapstructure.Decode(raw, &opts); err != nil {
		return yamlOptions{}, errUtils.Build(errUtils.ErrVersionYAMLOptionsInvalid).
			WithCause(err).
			WithHint(`Quote a bare numeric path, e.g. path: "0", so YAML doesn't parse it as an integer`).
			Err()
	}
	if dup := duplicatePath(opts.Set); dup != "" {
		return yamlOptions{}, fmt.Errorf("%w: %q", errUtils.ErrVersionYAMLDuplicatePath, dup)
	}
	return opts, nil
}

// duplicatePath returns the first path targeted by more than one set entry,
// or "" if every path is unique. Two entries targeting the same path would
// otherwise silently last-win with no indication the first write was
// discarded.
func duplicatePath(entries []setEntry) string {
	paths := make([]string, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
	}
	return managers.DuplicatePath(paths)
}

// applySets writes every configured set entry into content, skipping entries
// whose dependency is not locked (same skip-silently idiom as the marker,
// github-actions, and json managers).
func applySets(content []byte, entries []setEntry, refs map[string]manager.VersionRef, delims []string) ([]byte, error) {
	current := content
	for _, entry := range entries {
		ref, ok := refs[entry.From]
		if !ok || ref.Version == "" {
			continue
		}
		value := ref.String()
		if entry.Format != "" {
			formatted, err := managers.RenderValueFormat(entry.Format, ref, delims)
			if err != nil {
				return nil, fmt.Errorf("%w: path %q: %w", errUtils.ErrVersionYAMLFormatInvalid, entry.Path, err)
			}
			value = formatted
		}
		updated, err := applySet(current, entry, value)
		if err != nil {
			return nil, err
		}
		current = updated
	}
	return current, nil
}

// applySet writes one set entry's value into content via Atmos's
// format-preserving YAML editor (pkg/yaml), or returns content unchanged
// when the value already matches (avoiding an unnecessary re-render and
// strengthening idempotency). It rejects writing a scalar over a path whose
// current value is a map or list.
func applySet(content []byte, entry setEntry, value string) ([]byte, error) {
	existing, err := atmosyaml.Get(content, entry.Path)
	switch {
	case err == nil && existing == value:
		return content, nil
	case err != nil && !errors.Is(err, atmosyaml.ErrYAMLPathNotFound):
		return nil, fmt.Errorf("%w: path %q: %w", errUtils.ErrVersionYAMLSetFailed, entry.Path, err)
	}
	if t, ok := atmosyaml.GetType(content, entry.Path); ok && t == atmosyaml.TypeYAML {
		return nil, fmt.Errorf("%w: path %q", errUtils.ErrVersionYAMLPathTypeMismatch, entry.Path)
	}
	updated, err := atmosyaml.Set(content, entry.Path, value)
	if err != nil {
		return nil, fmt.Errorf("%w: path %q: %w", errUtils.ErrVersionYAMLSetFailed, entry.Path, err)
	}
	return updated, nil
}

func init() {
	managers.Register(Manager{})
}
