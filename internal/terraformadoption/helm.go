package terraformadoption

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/cli"
	"k8s.io/apimachinery/pkg/api/validation"

	errUtils "github.com/cloudposse/atmos/errors"
	helm "github.com/cloudposse/atmos/pkg/component/helm"
)

// helmEntry preserves Helm's release identity and ordered local values files.
func (r *Registration) helmEntry(dir, base string) (map[string]any, error) {
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
	source, err := relativeDirectory(base, dir)
	if err != nil {
		return nil, err
	}
	files, err := r.helmValuesFiles(dir)
	if err != nil {
		return nil, err
	}
	if err := helm.ValidateAdoptionInputs(dir, files); err != nil {
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

func (r *Registration) helmValuesFiles(dir string) ([]string, error) {
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
	if _, err := relativeDirectory(project, path); err != nil {
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
