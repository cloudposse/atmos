package kubernetes

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	sigsyaml "sigs.k8s.io/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// yamlDecodeBufferSize is the buffer size used by the YAML/JSON stream decoder.
const yamlDecodeBufferSize = 4096

type manifestLoader struct {
	componentPath string
	provider      string
}

func (l manifestLoader) Load(componentSection map[string]any) ([]*unstructured.Unstructured, error) {
	defer perf.Track(nil, "kubernetes.manifestLoader.Load")()

	objects := make([]*unstructured.Unstructured, 0)

	for _, manifest := range asAnySlice(componentSection["manifests"]) {
		loaded, err := l.loadManifestValue(manifest)
		if err != nil {
			return nil, err
		}
		objects = append(objects, loaded...)
	}

	paths := asStringSlice(componentSection["paths"])
	if len(paths) == 0 && len(objects) == 0 && l.componentPath != "" {
		if _, err := os.Stat(l.componentPath); err == nil {
			paths = []string{l.componentPath}
		}
	}

	for _, path := range paths {
		loaded, err := l.loadPath(path)
		if err != nil {
			return nil, err
		}
		objects = append(objects, loaded...)
	}

	return objects, nil
}

func (l manifestLoader) loadManifestValue(value any) ([]*unstructured.Unstructured, error) {
	switch typed := value.(type) {
	case string:
		return decodeObjects([]byte(typed))
	case map[string]any:
		data, err := sigsyaml.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal inline manifest: %w", err)
		}
		return decodeObjects(data)
	default:
		return nil, fmt.Errorf("%w, got %T", errUtils.ErrManifestEntryInvalidType, value)
	}
}

func (l manifestLoader) loadPath(path string) ([]*unstructured.Unstructured, error) {
	resolved := resolvePath(l.componentPath, path)
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to stat Kubernetes path %q: %w", path, err)
	}

	if !info.IsDir() {
		return loadManifestFile(resolved)
	}

	if l.provider == ProviderKustomize && hasKustomizationFile(resolved) {
		return renderKustomize(resolved)
	}

	files, err := manifestFilesInDir(resolved)
	if err != nil {
		return nil, err
	}

	objects := make([]*unstructured.Unstructured, 0)
	for _, file := range files {
		loaded, err := loadManifestFile(file)
		if err != nil {
			return nil, err
		}
		objects = append(objects, loaded...)
	}

	return objects, nil
}

func renderKustomize(path string) ([]*unstructured.Unstructured, error) {
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resMap, err := kustomizer.Run(filesys.MakeFsOnDisk(), path)
	if err != nil {
		return nil, fmt.Errorf("failed to render kustomize path %q: %w", path, err)
	}

	data, err := resMap.AsYaml()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize kustomize output for %q: %w", path, err)
	}

	return decodeObjects(data)
}

func loadManifestFile(path string) ([]*unstructured.Unstructured, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read Kubernetes manifest %q: %w", path, err)
	}
	return decodeObjects(data)
}

func decodeObjects(data []byte) ([]*unstructured.Unstructured, error) {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), yamlDecodeBufferSize)
	objects := make([]*unstructured.Unstructured, 0)

	for {
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to decode Kubernetes manifest: %w", err)
		}
		if len(raw) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{Object: raw}
		if obj.GetAPIVersion() == "" || obj.GetKind() == "" {
			return nil, errUtils.ErrManifestMissingAPIVersionKind
		}

		if obj.IsList() {
			if err := obj.EachListItem(func(item runtime.Object) error {
				itemObj, ok := item.(*unstructured.Unstructured)
				if !ok {
					return errUtils.ErrManifestListItemNotObject
				}
				objects = append(objects, itemObj)
				return nil
			}); err != nil {
				return nil, err
			}
			continue
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

func manifestFilesInDir(root string) ([]string, error) {
	files := make([]string, 0)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if isManifestFile(path) {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to walk Kubernetes manifest directory %q: %w", root, err)
	}
	sort.Strings(files)
	return files, nil
}

func isManifestFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func hasKustomizationFile(path string) bool {
	for _, name := range []string{"kustomization.yaml", "kustomization.yml", "Kustomization"} {
		if _, err := os.Stat(filepath.Join(path, name)); err == nil {
			return true
		}
	}
	return false
}

func resolvePath(basePath string, path string) string {
	if filepath.IsAbs(path) || basePath == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(basePath, path))
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
