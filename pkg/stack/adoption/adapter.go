// Package adoption discovers existing deployments and registers them in Atmos stacks.
package adoption

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Provider is an optional capability of a registered component provider. One
// component type can accept several source formats, such as an Atlantis project
// or a plain Terraform root.
type Provider interface {
	AdoptionAdapters() Adapters
}

// Adapter recognizes a source and registers a single selection. Detect only
// inspects the source; Adopt runs under the project lock (except for dry runs).
type Adapter interface {
	Kind() string
	Detect(Source) (Match, error)
	Options() []string
	Adopt(context.Context, *Registration, string) error
}

// Source describes a file or directory explicitly selected by the user. Files
// contains the visible file names directly inside a selected directory.
type Source struct {
	Path  string
	IsDir bool
	Files []string
}

// FileNames returns the visible configuration filenames in a directory listing.
func FileNames(entries []os.DirEntry) []string {
	defer perf.Track(nil, "adoption.FileNames")()

	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "~") {
			names = append(names, name)
		}
	}
	return names
}

// Match identifies the normalized adapter input. An empty Path declines the
// source. A wrapper owns the native files within it (e.g. a Terragrunt unit).
type Match struct {
	Path    string
	Wrapper bool
}

// Adapters contains the source adapters available to this invocation.
type Adapters []Adapter

// ErrNoMatch indicates that no adapter recognized the selected file or directory.
var ErrNoMatch = errors.New("no supported deployment configuration found")

type candidate struct {
	adapter Adapter
	path    string
}

// Detect selects the unique most-specific match independently of adapter order.
func (adapters Adapters) Detect(path string) (Adapter, string, error) {
	defer perf.Track(nil, "adoption.Adapters.Detect")()

	source, err := inspectSource(path)
	if err != nil {
		return nil, "", err
	}
	var native, wrappers []candidate
	for _, adapter := range adapters {
		match, err := adapter.Detect(source)
		if err != nil {
			return nil, "", err
		}
		if match.Path == "" {
			continue
		}
		c := candidate{adapter, match.Path}
		if match.Wrapper {
			wrappers = append(wrappers, c)
		} else {
			native = append(native, c)
		}
	}
	if len(wrappers) > 0 {
		native = wrappers
	}
	return selectMatch(path, native)
}

func selectMatch(path string, matches []candidate) (Adapter, string, error) {
	switch len(matches) {
	case 0:
		return nil, "", fmt.Errorf("%w: %w at %s", errUtils.ErrInvalidComponent, ErrNoMatch, path)
	case 1:
		return matches[0].adapter, matches[0].path, nil
	default:
		var kinds []string
		for _, c := range matches {
			kinds = append(kinds, c.adapter.Kind())
		}
		slices.Sort(kinds)
		return nil, "", fmt.Errorf("%w: %s contains multiple tool configurations (%s); select a specific configuration file or component directory", errUtils.ErrInvalidComponent, path, strings.Join(kinds, ", "))
	}
}

func inspectSource(path string) (Source, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Source{}, err
	}
	source := Source{Path: path, IsDir: info.IsDir()}
	if info.Mode().IsRegular() {
		return source, nil
	}
	if !source.IsDir {
		return source, fmt.Errorf("%w: expected a regular file at %s", errUtils.ErrInvalidComponent, path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return source, err
	}
	source.Files = FileNames(entries)
	return source, nil
}

// ValidName reports whether name is usable for a standalone stack or explicit component name.
func ValidName(name string) bool {
	defer perf.Track(nil, "adoption.ValidName")()
	return stackNamePattern.MatchString(name)
}

func (r *Registration) adopt(ctx context.Context) error {
	adapter, path, err := r.Adapters.Detect(r.Dir)
	if err != nil {
		return err
	}
	if err := r.CheckOptions(adapter); err != nil {
		return err
	}
	return adapter.Adopt(ctx, r, path)
}

// CheckOptions rejects explicit options unsupported by the selected source.
func (r *Registration) CheckOptions(adapter Adapter) error {
	defer perf.Track(nil, "adoption.Registration.CheckOptions")()

	if r.Name != "" && !ValidName(r.Name) {
		return fmt.Errorf("%w: invalid component name %q", errUtils.ErrInvalidComponent, r.Name)
	}
	allowed := adapter.Options()
	for _, option := range []struct {
		name string
		set  bool
	}{
		{"name", r.Name != ""},
		{"command", r.Explicit != ""},
		{"project", r.AtlantisProject != ""},
		{"var-file", len(r.VarFiles) > 0},
		{"namespace", r.Namespace != ""},
		{"release-name", r.ReleaseName != ""},
		{"values", len(r.Values) > 0},
	} {
		if option.set && !slices.Contains(allowed, option.name) {
			return fmt.Errorf("%w: --%s is unsupported for %s sources", errUtils.ErrInvalidFlagValue, option.name, adapter.Kind())
		}
	}
	return nil
}
