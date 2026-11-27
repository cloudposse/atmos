package adoption

import (
	"cmp"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

// WriteFiles applies the registration's dry-run policy to prepared files.
func (r *Registration) WriteFiles(files ...*File) error {
	defer perf.Track(nil, "adoption.Registration.WriteFiles")()

	for _, file := range files {
		if r.DryRun {
			rel, err := filepath.Rel(r.Project, file.Path)
			if err != nil {
				return err
			}
			printChange(r.Output, filepath.ToSlash(rel), file.Before, file.After)
		} else if err := file.Write(); err != nil {
			return err
		}
	}
	return nil
}

type registrationLayout struct {
	project         *File
	base, stacksDir string
}

func (r *Registration) localLayout(kind, dir, configuredBase string) (registrationLayout, error) {
	project, err := OpenFile(filepath.Join(r.Project, "atmos.yaml"))
	if err != nil {
		return registrationLayout{}, err
	}
	layout := registrationLayout{project: project, stacksDir: filepath.Join(r.Project, "stacks")}
	if r.Config == nil {
		if project.Before != nil {
			return layout, fmt.Errorf("%w: configuration appeared; reload and retry", errUtils.ErrInvalidStackConfig)
		}
		project.After = []byte(StarterConfig)
		layout.base, err = r.setLocalBase(project, kind, dir)
		return layout, err
	}
	if !StandaloneStacks(&r.Config.Stacks) {
		return layout, fmt.Errorf("%w: stack add requires standalone filename-named stacks", errUtils.ErrInvalidStackConfig)
	}
	layout.stacksDir = r.Config.StacksBaseAbsolutePath
	layout.base, err = r.configuredLocalBase(kind, configuredBase, project)
	if err == nil && layout.base == "" {
		layout.base, err = r.setLocalBase(project, kind, dir)
	}
	return layout, err
}

func resolveBase(project, base string) string {
	if base != "" && !filepath.IsAbs(base) {
		return filepath.Join(project, base)
	}
	return base
}

func (r *Registration) configuredLocalBase(kind, base string, project *File) (string, error) {
	base = resolveBase(r.Project, base)
	canonical, err := canonicalDestinationDir(base)
	if base != "" && err != nil {
		return "", err
	}
	if base != "" {
		if _, err := os.Stat(canonical); !os.IsNotExist(err) {
			return canonical, err
		}
	}
	_, explicit := y.GetType(project.Before, "components."+y.QuotePathSegment(kind)+".base_path")
	if explicit || project.Before == nil || (base != "" && canonical != filepath.Join(r.Project, "components", kind)) {
		return "", fmt.Errorf("%w: missing configured %s component base %s", errUtils.ErrInvalidStackConfig, kind, base)
	}
	return "", nil
}

func (r *Registration) setLocalBase(project *File, kind, dir string) (string, error) {
	if _, err := RelativeDirectory(r.Project, dir); err != nil {
		return "", err
	}
	base := filepath.Dir(dir)
	if dir == r.Project {
		base = r.Project
	}
	rel, err := filepath.Rel(r.Project, base)
	if err != nil {
		return "", err
	}
	err = project.Set("components."+y.QuotePathSegment(kind)+".base_path", filepath.ToSlash(rel), y.TypeString)
	return base, err
}

// RelativeDirectory returns a slash-separated path under a canonical component base.
func RelativeDirectory(base, dir string) (string, error) {
	defer perf.Track(nil, "adoption.RelativeDirectory")()

	base, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(base, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: %s is outside component base %s", errUtils.ErrInvalidComponent, dir, base)
	}
	return filepath.ToSlash(rel), nil
}

type standaloneManifest struct {
	Name       string
	Import     []any
	Components map[string]map[string]yaml.Node
	Shared     map[string]any `yaml:",inline"`
}

func (manifest *standaloneManifest) accepts(stack string) bool {
	return len(manifest.Import) == 0 && len(manifest.Shared) == 0 && (manifest.Name == "" || manifest.Name == stack)
}

func (r *Registration) insertLocalEntry(file *File, component *LocalComponent, name, dir string, entry map[string]any) error {
	var manifest standaloneManifest
	if err := yaml.Unmarshal(file.Before, &manifest); err != nil {
		return err
	}
	if !manifest.accepts(r.Stack) {
		return fmt.Errorf("%w: %s is not a standalone manifest for %s", errUtils.ErrInvalidStackConfig, file.Path, r.Stack)
	}
	dest := y.ComponentPath(component.Kind, name, "")
	current, exists := manifest.Components[component.Kind][name]
	if exists {
		if err := verifyLocalEntry(&current, entry, name, file.Path, len(component.VarFileExtensions) > 0); err != nil {
			return err
		}
	} else {
		data, err := setYAMLData(file.After, dest, entry)
		if err != nil {
			return err
		}
		file.After = data
	}
	if len(component.VarFileExtensions) == 0 {
		return nil
	}
	data, err := r.AddVarFiles(file.After, VarFileTarget{dest, file.Path, dir, component.VarFileExtensions}, exists)
	if err == nil {
		file.After = data
	}
	return err
}

func verifyLocalEntry(current *yaml.Node, entry map[string]any, name, path string, orderedVars bool) error {
	var fields map[string]any
	if err := current.Decode(&fields); err != nil {
		return err
	}
	if orderedVars {
		delete(fields, "vars")
	}
	var expected map[string]any
	raw, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(raw, &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, fields) {
		return fmt.Errorf("%w: component %s already exists with different settings in %s", errUtils.ErrInvalidComponent, name, path)
	}
	return nil
}

// VarFileTarget identifies the component field and its owning manifest on disk.
type VarFileTarget struct {
	Dest, File, Dir string
	Extensions      []string
}

// Bind a serialized YAML value as data. Paths in an include cannot become yq expressions.
func setYAMLData(content []byte, path string, value any) ([]byte, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return y.SetRaw(content, path, "(\""+encoded+"\" | @base64d | from_yaml)")
}

// LocalComponent describes one source-backed entry in a standalone stack. Entry
// receives the canonical source directory and resolved component base path.
type LocalComponent struct {
	Kind, Name, Dir, ConfiguredBase, NextCommand string
	VarFileExtensions                            []string
	Entry                                        func(dir, base string) (map[string]any, error)
}

func (component *LocalComponent) validate() error {
	if component == nil || component.Entry == nil || component.Kind == "" {
		return fmt.Errorf("%w: local component registration requires a kind and a manifest entry builder", errUtils.ErrInvalidComponent)
	}
	return nil
}

// WriteLocalComponent creates or extends a standalone manifest for a local
// component and preserves the project's existing component layout on retry.
func (r *Registration) WriteLocalComponent(component *LocalComponent) error {
	defer perf.Track(r.Config, "adoption.Registration.WriteLocalComponent")()

	if err := component.validate(); err != nil {
		return err
	}
	dir, err := filepath.EvalSymlinks(component.Dir)
	if err != nil {
		return err
	}
	layout, err := r.localLayout(component.Kind, dir, component.ConfiguredBase)
	if err != nil {
		return err
	}
	path, err := StandaloneDestination(layout.stacksDir, r.Stack)
	if err != nil {
		return err
	}
	manifest, err := OpenFile(path)
	if err != nil {
		return err
	}
	entry, err := component.Entry(dir, layout.base)
	if err != nil {
		return err
	}
	name := cmp.Or(component.Name, filepath.Base(dir))
	if err := r.insertLocalEntry(manifest, component, name, dir, entry); err != nil {
		return err
	}
	if err := r.WriteFiles(layout.project, manifest); err != nil {
		return err
	}
	fmt.Fprintf(r.Output, "Atmos stack: %s\nComponent: %s\nNext: atmos %s %s %s -s %s\n", r.Stack, name, component.Kind, component.NextCommand, name, r.Stack)
	return nil
}
