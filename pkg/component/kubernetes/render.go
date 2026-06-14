package kubernetes

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
)

// fileNameSep separates the parts of a generated manifest file name.
const fileNameSep = "_"

type renderOptions struct {
	Output    string
	OutputDir string
	Split     bool
}

func resolveRenderOptions(flags map[string]any, componentSection map[string]any) renderOptions {
	options := renderOptionsFromComponent(componentSection)

	if value, ok := flags["output"].(string); ok && value != "" {
		options.Output = value
		options.OutputDir = ""
	}
	if value, ok := flags["output_dir"].(string); ok && value != "" {
		options.OutputDir = value
		options.Output = ""
	}
	if value, ok := flags["split"].(bool); ok && value {
		options.Split = true
	}

	return options
}

func renderOptionsFromComponent(componentSection map[string]any) renderOptions {
	renderSection, ok := componentSection["render"].(map[string]any)
	if !ok {
		return renderOptions{}
	}
	outputSection, ok := renderSection["output"].(map[string]any)
	if !ok {
		return renderOptions{}
	}

	options := renderOptions{}
	if split, ok := outputSection["split"].(bool); ok {
		options.Split = split
	}
	if path, ok := outputSection["path"].(string); ok && path != "" {
		if options.Split {
			options.OutputDir = path
		} else {
			options.Output = path
		}
	}

	return options
}

func renderObjects(objects []*unstructured.Unstructured, options renderOptions) error {
	if err := validateRenderOptions(options); err != nil {
		return err
	}
	return dispatchRender(objects, options)
}

// validateRenderOptions verifies the render output flag combination is valid.
func validateRenderOptions(options renderOptions) error {
	if options.Output != "" && options.OutputDir != "" {
		return errUtils.ErrKubernetesOutputDirMutuallyExclusive
	}
	if options.Output != "" && options.Split {
		return errUtils.ErrKubernetesSplitRequiresOutputDir
	}
	if options.Split && options.OutputDir == "" {
		return errUtils.ErrKubernetesSplitNeedsOutputDir
	}
	return nil
}

// dispatchRender writes the rendered objects according to the resolved output mode.
func dispatchRender(objects []*unstructured.Unstructured, options renderOptions) error {
	switch {
	case options.Output != "":
		return writeSingleManifestFile(options.Output, objects)
	case options.OutputDir != "" && options.Split:
		return writeSplitManifestFiles(options.OutputDir, objects)
	case options.OutputDir != "":
		return writeSingleManifestFile(filepath.Join(options.OutputDir, "manifest.yaml"), objects)
	default:
		data, renderErr := multiDocumentYAML(objects)
		if renderErr != nil {
			return renderErr
		}
		_, err := os.Stdout.Write(data)
		return err
	}
}

func writeSingleManifestFile(path string, objects []*unstructured.Unstructured) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("failed to create render output directory for %q: %w", path, err)
	}
	data, err := multiDocumentYAML(objects)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("failed to write rendered manifests to %q: %w", path, err)
	}
	fmt.Fprintf(os.Stdout, "rendered %d Kubernetes object(s) to %s\n", len(objects), path)
	return nil
}

func writeSplitManifestFiles(outputDir string, objects []*unstructured.Unstructured) error {
	if err := os.MkdirAll(outputDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create render output directory %q: %w", outputDir, err)
	}

	for i, obj := range objects {
		path := filepath.Join(outputDir, objectManifestFileName(i, obj))
		data, err := objectYAML(obj)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, filePerm); err != nil {
			return fmt.Errorf("failed to write rendered manifest %q: %w", path, err)
		}
	}

	fmt.Fprintf(os.Stdout, "rendered %d Kubernetes object(s) to %s\n", len(objects), outputDir)
	return nil
}

func multiDocumentYAML(objects []*unstructured.Unstructured) ([]byte, error) {
	var buffer bytes.Buffer
	for i, obj := range objects {
		if i > 0 {
			buffer.WriteString("---\n")
		}
		data, err := objectYAML(obj)
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

func objectYAML(obj *unstructured.Unstructured) ([]byte, error) {
	data, err := sigsyaml.Marshal(obj.Object)
	if err != nil {
		return nil, fmt.Errorf("failed to render %s/%s: %w", obj.GetKind(), obj.GetName(), err)
	}
	return data, nil
}

func objectManifestFileName(index int, obj *unstructured.Unstructured) string {
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

var invalidFileNameChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func sanitizeFileNamePart(value string) string {
	value = strings.ReplaceAll(value, "/", "_")
	value = invalidFileNameChars.ReplaceAllString(value, "_")
	return strings.Trim(value, "_")
}
