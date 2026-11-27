package helm

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/cli"
	"k8s.io/apimachinery/pkg/api/validation"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
)

// helmEntry preserves Helm's release identity and ordered local values files.
func helmEntry(r *stackadoption.Registration, dir, base string) (map[string]any, error) {
	release := cmp.Or(r.ReleaseName, r.Name, filepath.Base(dir))
	if err := chartutil.ValidateReleaseName(release); err != nil {
		return nil, fmt.Errorf("release %q: %w", release, err)
	}
	namespace := r.Namespace
	if namespace == "" {
		namespace = cli.New().Namespace()
	}
	if problems := validation.ValidateNamespaceName(namespace, false); len(problems) > 0 {
		return nil, fmt.Errorf("%w: namespace %q: %s", errUtils.ErrInvalidFlagValue, namespace, strings.Join(problems, ", "))
	}
	source, err := stackadoption.RelativeDirectory(base, dir)
	if err != nil {
		return nil, err
	}
	files, err := helmValuesFiles(r, dir)
	if err != nil {
		return nil, err
	}
	if err := ValidateAdoptionInputs(dir, files); err != nil {
		return nil, err
	}

	entry := map[string]any{
		"chart": ".", "name": release, "namespace": namespace,
		"metadata": map[string]any{"component": source},
	}
	if len(files) > 0 {
		entry["values_files"] = files
	}
	return entry, nil
}

func helmValuesFiles(r *stackadoption.Registration, dir string) ([]string, error) {
	var paths []string
	for _, value := range r.Values {
		path, err := helmValuesPath(r.Project, dir, value)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func helmValuesPath(project, chartDir, value string) (string, error) {
	path, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if _, err := stackadoption.RelativeDirectory(project, path); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: expected a regular local values file at %s", errUtils.ErrInvalidFlagValue, path)
	}
	relative, err := filepath.Rel(chartDir, path)
	if strings.Contains(relative, "{{") {
		return "", fmt.Errorf("%w: %q contains a template delimiter in the saved values path", errUtils.ErrInvalidFlagValue, relative)
	}
	return filepath.ToSlash(relative), err
}

// AdoptionAdapters exposes local chart registration through the component registry.
func (*ComponentProvider) AdoptionAdapters() stackadoption.Adapters {
	defer perf.Track(nil, "helm.ComponentProvider.AdoptionAdapters")()

	return stackadoption.Adapters{chartAdapter{}}
}

type chartAdapter struct{}

func (chartAdapter) Kind() string {
	defer perf.Track(nil, "helm.chartAdapter.Kind")()
	return "helm"
}

func (chartAdapter) Options() []string {
	defer perf.Track(nil, "helm.chartAdapter.Options")()

	return []string{"name", "values", "release-name", "namespace"}
}

func (chartAdapter) Detect(s stackadoption.Source) (stackadoption.Match, error) {
	defer perf.Track(nil, "helm.chartAdapter.Detect")()

	if s.IsDir && slices.Contains(s.Files, "Chart.yaml") {
		return stackadoption.Match{Path: s.Path}, nil
	}
	if !s.IsDir && filepath.Base(s.Path) == "Chart.yaml" {
		return stackadoption.Match{Path: filepath.Dir(s.Path)}, nil
	}
	return stackadoption.Match{}, nil
}

func (a chartAdapter) Adopt(_ context.Context, r *stackadoption.Registration, dir string) error {
	defer perf.Track(nil, "helm.chartAdapter.Adopt")()

	base := ""
	if r.Config != nil {
		base = r.Config.HelmDirAbsolutePath
	}
	return r.WriteLocalComponent(&stackadoption.LocalComponent{
		Kind:           a.Kind(),
		Name:           cmp.Or(r.Name, r.ReleaseName),
		Dir:            dir,
		ConfiguredBase: base,
		NextCommand:    "template",
		Entry:          func(dir, base string) (map[string]any, error) { return helmEntry(r, dir, base) },
	})
}
