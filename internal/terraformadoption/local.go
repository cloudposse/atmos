package terraformadoption

import (
	"cmp"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/stack"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

const (
	atlantisKind = "atlantis"
	helmKind     = "helm"
	packerKind   = "packer"
	chartFile    = "Chart.yaml"
)

type localSource struct{ kind, path string }

func detectSource(path string) (localSource, error) {
	info, err := os.Stat(path)
	if err != nil {
		return localSource{}, err
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return localSource{}, fmt.Errorf("%w: expected a regular file at %s", errUtils.ErrInvalidComponent, path)
		}
		return fileSource(path)
	}
	unit, err := TerragruntUnit(path)
	if err != nil {
		return localSource{}, err
	}
	if unit != "" {
		return localSource{tg, unit}, nil
	}
	files, err := os.ReadDir(path)
	if err != nil {
		return localSource{}, err
	}
	return directorySource(path, files)
}

func fileSource(path string) (localSource, error) {
	switch name := filepath.Base(path); {
	case name == "atlantis.yaml" || name == "atlantis.yml":
		return localSource{atlantisKind, path}, nil
	case isPackerTemplate(name):
		return localSource{packerKind, path}, nil
	case name == chartFile:
		return localSource{helmKind, filepath.Dir(path)}, nil
	case name == unitFile || name == stackFile:
		unit, err := TerragruntUnit(path)
		return localSource{tg, unit}, err
	default:
		return localSource{}, fmt.Errorf("%w: select a tool directory or a supported configuration file at %s", errUtils.ErrInvalidComponent, path)
	}
}

func directorySource(path string, files []os.DirEntry) (localSource, error) {
	kinds := make(map[string]bool)
	for _, f := range files {
		if ignoredSource(f) {
			continue
		}
		switch name := f.Name(); {
		case name == chartFile:
			kinds[helmKind] = true
		case isPackerTemplate(name):
			kinds[packerKind] = true
		case configSuffix(name, ".tf") || configSuffix(name, ".tofu"):
			kinds[terraform] = true
		}
	}
	if len(kinds) > 1 {
		return localSource{}, fmt.Errorf("%w: %s contains multiple tool configurations; select the chart or Packer template file, or a single Terraform root", errUtils.ErrInvalidComponent, path)
	}
	for kind := range kinds {
		return localSource{kind, path}, nil
	}
	return localSource{terraform, path}, nil // Keep the Terraform selector's diagnostic for an empty root.
}

func (r *Registration) checkSourceFlags(kind string) error {
	switch {
	case r.Explicit != "" && (kind == helmKind || kind == packerKind):
		return fmt.Errorf("%w: --command selects the Terraform or OpenTofu executable", errUtils.ErrInvalidFlagValue)
	case r.AtlantisProject != "" && kind != atlantisKind:
		return fmt.Errorf("%w: --project requires an Atlantis config", errUtils.ErrInvalidFlagValue)
	case r.Name != "" && !stackNamePattern.MatchString(r.Name):
		return fmt.Errorf("%w: invalid component name %q", errUtils.ErrInvalidComponent, r.Name)
	case kind != helmKind && r.hasHelmOptions():
		return fmt.Errorf("%w: --values, --namespace and --release-name require a Helm chart", errUtils.ErrInvalidFlagValue)
	}
	return r.checkToolOptions(kind)
}

func (r *Registration) hasHelmOptions() bool {
	return len(r.Values) > 0 || r.Namespace != "" || r.ReleaseName != ""
}

func (r *Registration) checkToolOptions(kind string) error {
	switch kind {
	case atlantisKind:
		if len(r.VarFiles) > 0 {
			return fmt.Errorf("%w: the selected Atlantis workflow supplies its variable files", errUtils.ErrInvalidFlagValue)
		}
	case helmKind:
		if len(r.VarFiles) > 0 {
			return fmt.Errorf("%w: use --values for Helm charts", errUtils.ErrInvalidFlagValue)
		}
	case tg:
		if len(r.VarFiles) > 0 || r.Name != "" {
			return fmt.Errorf("%w: a Terragrunt unit supplies its component name and variables through HCL", errUtils.ErrInvalidFlagValue)
		}
	}
	return nil
}

func (s localSource) directory() (string, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return filepath.EvalSymlinks(s.path)
	}
	return filepath.EvalSymlinks(filepath.Dir(s.path))
}

func (r *Registration) saveLocalTool(source localSource) error {
	dir, err := source.directory()
	if err != nil {
		return err
	}
	layout, err := r.localLayout(source.kind, dir)
	if err != nil {
		return err
	}
	path, err := standaloneDestination(layout.stacksDir, r.Stack)
	if err != nil {
		return err
	}
	manifest, err := openAdoptionFile(path)
	if err != nil {
		return err
	}
	name := cmp.Or(r.Name, r.ReleaseName, filepath.Base(dir))
	entry, err := r.localEntry(source, dir, layout.base)
	if err != nil {
		return err
	}
	if err := r.insertLocalEntry(manifest, source.kind, name, entry, dir); err != nil {
		return err
	}
	if err := r.writeLocalFiles(layout.project, manifest); err != nil {
		return err
	}
	next := "template"
	if source.kind == packerKind {
		next = "validate"
	}
	fmt.Fprintf(r.Output, "Atmos stack: %s\nComponent: %s\nNext: atmos %s %s %s -s %s\n", r.Stack, name, source.kind, next, name, r.Stack)
	return nil
}

func (r *Registration) writeLocalFiles(files ...*adoptionFile) error {
	for _, file := range files {
		if r.DryRun {
			rel, err := filepath.Rel(r.Project, file.path)
			if err != nil {
				return err
			}
			printChange(r.Output, filepath.ToSlash(rel), file.before, file.after)
		} else if err := file.write(); err != nil {
			return err
		}
	}
	return nil
}

type registrationLayout struct {
	project         *adoptionFile
	base, stacksDir string
}

func (r *Registration) localLayout(kind, dir string) (registrationLayout, error) {
	project, err := openAdoptionFile(filepath.Join(r.Project, "atmos.yaml"))
	if err != nil {
		return registrationLayout{}, err
	}
	layout := registrationLayout{project: project, stacksDir: filepath.Join(r.Project, "stacks")}
	if r.Config == nil {
		if project.before != nil {
			return layout, fmt.Errorf("%w: configuration appeared; reload and retry", errUtils.ErrInvalidStackConfig)
		}
		project.after = []byte(starter)
		layout.base, err = r.setLocalBase(project, kind, dir)
		return layout, err
	}
	if !standaloneStacks(&r.Config.Stacks) {
		return layout, fmt.Errorf("%w: stack add requires standalone filename-named stacks", errUtils.ErrInvalidStackConfig)
	}
	layout.stacksDir = r.Config.StacksBaseAbsolutePath
	layout.base, err = r.configuredLocalBase(kind, project)
	if err == nil && layout.base == "" {
		layout.base, err = r.setLocalBase(project, kind, dir)
	}
	return layout, err
}

func (r *Registration) configuredLocalBase(kind string, project *adoptionFile) (string, error) {
	base := r.Config.HelmDirAbsolutePath
	if kind == packerKind {
		base = r.Config.PackerDirAbsolutePath
	}
	canonical, err := canonicalDestinationDir(base)
	if base != "" && err != nil {
		return "", err
	}
	if base != "" {
		if _, err := os.Stat(canonical); !os.IsNotExist(err) {
			return canonical, err
		}
	}
	_, explicit := y.GetType(project.before, "components."+kind+".base_path")
	if explicit || project.before == nil || (base != "" && canonical != filepath.Join(r.Project, "components", kind)) {
		return "", fmt.Errorf("%w: missing configured %s component base %s", errUtils.ErrInvalidStackConfig, kind, base)
	}
	return "", nil
}

func (r *Registration) setLocalBase(project *adoptionFile, kind, dir string) (string, error) {
	if _, err := relativeDirectory(r.Project, dir); err != nil {
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
	err = project.set("components."+kind+".base_path", filepath.ToSlash(rel), y.TypeString)
	return base, err
}

func relativeDirectory(base, dir string) (string, error) {
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

func (r *Registration) localEntry(s localSource, dir, base string) (map[string]any, error) {
	if s.kind == helmKind {
		return r.helmEntry(dir, base)
	}
	return packerEntry(s.path, dir, base)
}

type standaloneManifest struct {
	Name       string
	Import     []any
	Components map[string]map[string]yaml.Node
	Shared     map[string]any `yaml:",inline"`
}

func (r *Registration) insertLocalEntry(file *adoptionFile, kind, name string, entry map[string]any, dir string) error {
	var manifest standaloneManifest
	if err := yaml.Unmarshal(file.before, &manifest); err != nil {
		return err
	}
	if len(manifest.Import) > 0 || len(manifest.Shared) > 0 || (manifest.Name != "" && manifest.Name != r.Stack) {
		return fmt.Errorf("%w: %s is not a standalone manifest for %s", errUtils.ErrInvalidStackConfig, file.path, r.Stack)
	}
	dest := stack.BuildComponentYqPath(kind, name, "")
	current, exists := manifest.Components[kind][name]
	if exists {
		if err := verifyLocalEntry(&current, entry, name, file.path); err != nil {
			return err
		}
	} else {
		data, err := setYAMLData(file.after, dest, entry)
		if err != nil {
			return err
		}
		file.after = data
	}
	data, err := r.addVarFiles(file.after, varFileTarget{dest, file.path, dir, kind}, exists)
	if err == nil {
		file.after = data
	}
	return err
}

func verifyLocalEntry(current *yaml.Node, entry map[string]any, name, path string) error {
	var fields map[string]any
	if err := current.Decode(&fields); err != nil {
		return err
	}
	delete(fields, "vars")
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

// varFileTarget identifies the component field and its owning manifest on disk.
type varFileTarget struct{ dest, file, dir, kind string }

// Bind a serialized YAML value as data. Paths in an include cannot become yq expressions.
func setYAMLData(content []byte, path string, value any) ([]byte, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return y.SetRaw(content, path, "(\""+encoded+"\" | @base64d | from_yaml)")
}

func isPackerTemplate(name string) bool {
	return strings.HasSuffix(name, ".pkr.hcl") || strings.HasSuffix(name, ".pkr.json")
}
