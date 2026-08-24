package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultNamespace = "default"

// buildChartSpec assembles the resolved chart specification from the merged
// component section. Local chart references are resolved relative to the
// component path.
func buildChartSpec(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentPath string,
) (*chartSpec, error) {
	defer perf.Track(atmosConfig, "helm.buildChartSpec")()

	section := info.ComponentSection

	chart, _ := section[cfg.ChartSectionName].(string)
	chart = resolveLocalChart(chart, componentPath)

	values, err := buildValues(atmosConfig, section, componentPath)
	if err != nil {
		return nil, err
	}

	return &chartSpec{
		Chart:        chart,
		RepoURL:      stringField(section, "repository"),
		Version:      stringField(section, "version"),
		ReleaseName:  resolveReleaseName(section, info),
		Namespace:    resolveNamespace(section),
		Values:       values,
		IncludeCRDs:  true,
		Repositories: mergeRepositories(atmosConfig, section),
	}, nil
}

// buildValues merges the inline `values:` map with any `values_files:` overlays.
// Precedence (low to high): values_files in listed order, then inline values.
func buildValues(atmosConfig *schema.AtmosConfiguration, section map[string]any, componentPath string) (map[string]any, error) {
	layers := make([]map[string]any, 0)

	for _, file := range asStringSlice(section[cfg.ValuesFilesSectionName]) {
		loaded, err := loadValuesFile(resolveLocalChart(file, componentPath))
		if err != nil {
			return nil, err
		}
		layers = append(layers, loaded)
	}

	if inline, ok := section[cfg.ValuesSectionName].(map[string]any); ok {
		layers = append(layers, inline)
	}

	if len(layers) == 0 {
		return map[string]any{}, nil
	}

	merged, err := merge.Merge(atmosConfig, layers)
	if err != nil {
		return nil, fmt.Errorf("failed to merge Helm values: %w", err)
	}
	return merged, nil
}

func loadValuesFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read Helm values file %q: %w", path, err)
	}
	var out map[string]any
	if err := sigsyaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse Helm values file %q: %w", path, err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// resolveLocalChart resolves a relative chart/value path against the component
// path. Absolute paths, OCI refs, and repo/name references pass through. A
// reference is treated as local when it begins with "./" or "../", or it
// resolves on disk relative to the component path.
func resolveLocalChart(ref, componentPath string) string {
	if ref == "" || componentPath == "" || filepath.IsAbs(ref) {
		return ref
	}
	if strings.HasPrefix(ref, ".") || pathExistsRelative(componentPath, ref) {
		return filepath.Join(componentPath, ref)
	}
	return ref
}

func pathExistsRelative(base, ref string) bool {
	_, err := os.Stat(filepath.Join(base, ref))
	return err == nil
}

func resolveReleaseName(section map[string]any, info *schema.ConfigAndStacksInfo) string {
	if name := stringField(section, "name"); name != "" {
		return name
	}
	// Use the last path segment of the component name as the release name.
	return filepath.Base(info.ComponentFromArg)
}

func resolveNamespace(section map[string]any) string {
	if ns := stringField(section, "namespace"); ns != "" {
		return ns
	}
	return defaultNamespace
}

// repositoriesMap converts a `repositories:` list of {name,url} into a name->url
// map for callers that only need chart reference lookup.
func repositoriesMap(section map[string]any) map[string]string {
	out := make(map[string]string)
	repositories := repositoriesFromSection(section, repositorySourceComponent)
	for i := range repositories {
		repo := &repositories[i]
		out[repo.Name] = repo.URL
	}
	return out
}

func mergeRepositories(atmosConfig *schema.AtmosConfiguration, section map[string]any) []chartRepository {
	out := make([]chartRepository, 0)
	positions := make(map[string]int)

	global := globalRepositories(atmosConfig)
	for i := range global {
		repo := &global[i]
		if repo.Name == "" || repo.URL == "" {
			continue
		}
		positions[repo.Name] = len(out)
		out = append(out, *repo)
	}

	componentRepositories := repositoriesFromSection(section, repositorySourceComponent)
	for i := range componentRepositories {
		repo := &componentRepositories[i]
		if repo.Name == "" || repo.URL == "" {
			continue
		}
		if pos, ok := positions[repo.Name]; ok {
			out[pos] = *repo
			continue
		}
		positions[repo.Name] = len(out)
		out = append(out, *repo)
	}

	return out
}

func globalRepositories(atmosConfig *schema.AtmosConfiguration) []chartRepository {
	if atmosConfig == nil {
		return nil
	}
	out := make([]chartRepository, 0, len(atmosConfig.Components.Helm.Repositories))
	for i := range atmosConfig.Components.Helm.Repositories {
		repo := &atmosConfig.Components.Helm.Repositories[i]
		out = append(out, chartRepository{
			Name:                  repo.Name,
			URL:                   repo.URL,
			Username:              repo.Username,
			Password:              repo.Password,
			PassCredentialsAll:    repo.PassCredentialsAll,
			CertFile:              repo.CertFile,
			KeyFile:               repo.KeyFile,
			CAFile:                repo.CAFile,
			InsecureSkipTLSVerify: repo.InsecureSkipTLSVerify,
			Source:                repositorySourceGlobal,
		})
	}
	return out
}

func repositoriesFromSection(section map[string]any, source repositorySource) []chartRepository {
	out := make([]chartRepository, 0)
	for _, entry := range asAnySlice(section[cfg.RepositoriesSectionName]) {
		repo, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		item := chartRepository{
			Name:                  stringField(repo, "name"),
			URL:                   stringField(repo, "url"),
			Username:              stringField(repo, "username"),
			Password:              stringField(repo, "password"),
			PassCredentialsAll:    boolField(repo, "pass_credentials_all"),
			CertFile:              stringField(repo, "cert_file"),
			KeyFile:               stringField(repo, "key_file"),
			CAFile:                stringField(repo, "ca_file"),
			InsecureSkipTLSVerify: boolField(repo, "insecure_skip_tls_verify"),
			Source:                source,
		}
		if item.Name != "" && item.URL != "" {
			out = append(out, item)
		}
	}
	return out
}

func findRepository(repositories []chartRepository, name string) (chartRepository, bool) {
	for i := range repositories {
		repo := &repositories[i]
		if repo.Name == name {
			return *repo, true
		}
	}
	return chartRepository{}, false
}

func stringField(section map[string]any, key string) string {
	value, _ := section[key].(string)
	return value
}

func boolField(section map[string]any, key string) bool {
	value, _ := section[key].(bool)
	return value
}

func asStringSlice(value any) []string {
	values := make([]string, 0)
	for _, item := range asAnySlice(value) {
		if str, ok := item.(string); ok && str != "" {
			values = append(values, str)
		}
	}
	return values
}

func asAnySlice(value any) []any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		return typed
	case []string:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	case string:
		return []any{typed}
	default:
		return nil
	}
}
