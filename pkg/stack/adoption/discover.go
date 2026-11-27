package adoption

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Candidate is an explicit path accepted by a registered source adapter.
type Candidate struct{ Path, Kind string }

const (
	scanRoot       = "."
	maxSourceDepth = 5
)

func skipDirectory(name string) bool {
	return strings.HasPrefix(name, ".") || slices.Contains([]string{"node_modules", "vendor", "target", "dist", "build", "__pycache__"}, name)
}

func visibleFile(e fs.DirEntry) bool {
	return e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".")
}

// Discover groups native files by root and skips caches and symlinks. Explicit paths have no depth limit.
func (adapters Adapters) Discover(ctx context.Context, root string) ([]Candidate, error) {
	defer perf.Track(nil, "adoption.Adapters.Discover")()
	var result []Candidate
	skip := map[string]bool{}
	err := fs.WalkDir(os.DirFS(root), scanRoot, func(rel string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !e.IsDir() {
			return nil
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		if skipSourceDirectory(rel, e.Name(), skip[path]) {
			return fs.SkipDir
		}
		candidates, err := adapters.candidatesIn(path)
		if err != nil {
			return err
		}
		for _, c := range candidates {
			if c.Kind == "helm" {
				skip[filepath.Join(path, "charts")] = true
			}
		}
		result = append(result, candidates...)
		return nil
	})
	return result, err
}

func skipSourceDirectory(rel, name string, vendored bool) bool {
	return rel != scanRoot && (skipDirectory(name) || strings.Count(rel, "/") >= maxSourceDepth || vendored)
}

// A directory provider owns its files. A separate wrapper file (e.g. atlantis.yaml)
// can represent the same directory while remaining selectable by its own path.
func (adapters Adapters) candidatesIn(path string) ([]Candidate, error) {
	var native, wrappers []Candidate
	kind := ""
	adapter, _, err := adapters.Detect(path)
	if err == nil {
		kind = adapter.Kind()
		native = append(native, Candidate{path, kind})
	} else if !errors.Is(err, errUtils.ErrInvalidComponent) {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !visibleFile(e) {
			continue
		}
		file := filepath.Join(path, e.Name())
		found, wrapped := adapters.fileCandidates(file, kind)
		native = append(native, found...)
		wrappers = append(wrappers, wrapped...)
	}
	if len(wrappers) > 0 {
		return wrappers, nil
	}
	return native, nil
}

func (adapters Adapters) fileCandidates(file, kind string) (native, wrappers []Candidate) {
	for _, a := range adapters {
		if a.Kind() == kind {
			continue
		}
		m, detectErr := a.Detect(Source{Path: file})
		if detectErr != nil || m.Path == "" {
			continue
		}
		switch {
		case m.Wrapper:
			wrappers = append(wrappers, Candidate{file, a.Kind()})
		case kind == "":
			native = append(native, Candidate{file, a.Kind()})
		}
	}
	return native, wrappers
}

// VariableInputs separates automatically loaded inputs from explicitly selectable files.
type VariableInputs struct{ Automatic, Optional []string }

type inputConvention struct{ templates, extensions, autoExtensions, autoNames []string }

var variableConventions = map[string]inputConvention{
	"terraform": {templates: []string{".tf", ".tf.json", ".tofu", ".tofu.json"}, extensions: []string{".tfvars", ".tfvars.json"}, autoExtensions: []string{".auto.tfvars", ".auto.tfvars.json"}, autoNames: []string{"terraform.tfvars", "terraform.tfvars.json"}},
	"packer":    {templates: []string{".pkr.hcl", ".pkr.json"}, extensions: []string{".pkrvars.hcl", ".pkrvars.json"}, autoExtensions: []string{".auto.pkrvars.hcl", ".auto.pkrvars.json"}},
}

func hasExtension(name string, suffixes []string) bool {
	return slices.ContainsFunc(suffixes, func(s string) bool { return strings.HasSuffix(name, s) })
}

func (c *inputConvention) descend(root fs.FS, dir string, e fs.DirEntry) error {
	if dir == scanRoot {
		return nil
	}
	if skipDirectory(e.Name()) {
		return fs.SkipDir
	}
	entries, err := fs.ReadDir(root, dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if visibleFile(entry) && hasExtension(entry.Name(), c.templates) {
			return fs.SkipDir
		}
	}
	return nil
}

// DiscoverVarFiles returns native-relative paths ordered only for display. A nested root owns its inputs.
func DiscoverVarFiles(source, kind string) (VariableInputs, error) {
	defer perf.Track(nil, "adoption.DiscoverVarFiles")()
	var inputs VariableInputs
	c, ok := variableConventions[kind]
	if !ok {
		return inputs, nil
	}
	info, err := os.Stat(source)
	if err != nil {
		return inputs, err
	}
	if !info.IsDir() {
		source = filepath.Dir(source)
	}
	root := os.DirFS(source)
	err = fs.WalkDir(root, scanRoot, func(rel string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return c.descend(root, rel, e)
		}
		if !visibleFile(e) || !hasExtension(e.Name(), c.extensions) {
			return nil
		}
		c.add(&inputs, rel, e.Name())
		return nil
	})
	return inputs, err
}

func (c *inputConvention) add(inputs *VariableInputs, rel, name string) {
	automatic := slices.Contains(c.autoNames, name) || hasExtension(name, c.autoExtensions)
	if automatic && rel == name {
		inputs.Automatic = append(inputs.Automatic, rel)
	} else {
		inputs.Optional = append(inputs.Optional, rel)
	}
}
