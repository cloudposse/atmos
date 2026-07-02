// Package manifest provides shared helpers for decoding, serializing, naming,
// and packaging Kubernetes manifest objects. It is producer-agnostic: rendered
// manifests from the Helm component, the Kubernetes component, or the Helmfile
// `template` path all flow through these helpers so they emit identical output
// and identical provision-artifact shapes for delivery targets (e.g. git).
package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// yamlDecodeBufferSize is the buffer size used by the YAML/JSON stream decoder.
const yamlDecodeBufferSize = 4096

// fileNameSep separates the parts of a generated manifest file name.
const fileNameSep = "_"

// DecodeObjects decodes a multi-document YAML/JSON byte stream into a slice of
// unstructured objects. Embedded Kubernetes List objects are expanded into their
// individual items.
func DecodeObjects(data []byte) ([]*unstructured.Unstructured, error) {
	defer perf.Track(nil, "manifest.DecodeObjects")()

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

// ObjectYAML serializes a single object to YAML.
func ObjectYAML(obj *unstructured.Unstructured) ([]byte, error) {
	defer perf.Track(nil, "manifest.ObjectYAML")()

	data, err := sigsyaml.Marshal(obj.Object)
	if err != nil {
		return nil, fmt.Errorf("failed to render %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}
	return data, nil
}

// MultiDocumentYAML serializes objects into a single multi-document YAML stream.
func MultiDocumentYAML(objects []*unstructured.Unstructured) ([]byte, error) {
	defer perf.Track(nil, "manifest.MultiDocumentYAML")()

	var buffer bytes.Buffer
	for i, obj := range objects {
		if i > 0 {
			buffer.WriteString("---\n")
		}
		data, err := ObjectYAML(obj)
		if err != nil {
			return nil, err
		}
		buffer.Write(data)
		if !bytes.HasSuffix(data, []byte("\n")) {
			buffer.WriteByte('\n')
		}
	}
	return buffer.Bytes(), nil
}

// ObjectFileName returns a deterministic, filesystem-safe file name for an
// object, prefixed with its 1-based position so ordering is preserved.
func ObjectFileName(index int, obj *unstructured.Unstructured) string {
	defer perf.Track(nil, "manifest.ObjectFileName")()

	parts := []string{
		fmt.Sprintf("%03d", index+1),
		obj.GroupVersionKind().Group,
		obj.GetAPIVersion(),
		obj.GetKind(),
		obj.GetNamespace(),
		obj.GetName(),
	}

	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, sanitizeFileNamePart(part))
		}
	}
	return strings.Join(nonEmpty, fileNameSep) + ".yaml"
}

// ArtifactFiles builds a deterministic per-object file set (name -> YAML bytes)
// suitable for delivery to a provision target.
func ArtifactFiles(objects []*unstructured.Unstructured) (map[string][]byte, error) {
	defer perf.Track(nil, "manifest.ArtifactFiles")()

	files := make(map[string][]byte, len(objects))
	for i, obj := range objects {
		data, err := ObjectYAML(obj)
		if err != nil {
			return nil, err
		}
		files[ObjectFileName(i, obj)] = data
	}
	return files, nil
}

var invalidFileNameChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func sanitizeFileNamePart(value string) string {
	value = strings.ReplaceAll(value, "/", "_")
	value = invalidFileNameChars.ReplaceAllString(value, "_")
	return strings.Trim(value, "_")
}
