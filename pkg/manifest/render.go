package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// The dirPerm const is the permission mode used when creating directories.
	dirPerm = 0o755
	// The filePerm const is the permission mode used when writing rendered manifest files.
	filePerm = 0o600
)

// RenderOptions controls how rendered manifests are written.
type RenderOptions struct {
	// Output writes all objects to a single multi-document YAML file.
	Output string
	// OutputDir writes objects to a directory (a single manifest.yaml unless Split is set).
	OutputDir string
	// Split writes one file per object. Requires OutputDir.
	Split bool
	// Noun is the human-readable object noun used in status output (e.g. "Helm", "Kubernetes").
	Noun string
}

// ValidateRenderOptions verifies the output flag combination is valid.
func ValidateRenderOptions(options RenderOptions) error {
	defer perf.Track(nil, "manifest.ValidateRenderOptions")()

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

// WriteObjects writes the rendered objects according to the resolved output mode.
// With no Output/OutputDir set, the multi-document YAML is written to stdout.
func WriteObjects(objects []*unstructured.Unstructured, options RenderOptions) error {
	defer perf.Track(nil, "manifest.WriteObjects")()

	if err := ValidateRenderOptions(options); err != nil {
		return err
	}
	switch {
	case options.Output != "":
		return writeSingleFile(options.Output, objects, options.Noun)
	case options.OutputDir != "" && options.Split:
		return writeSplitFiles(options.OutputDir, objects, options.Noun)
	case options.OutputDir != "":
		return writeSingleFile(filepath.Join(options.OutputDir, "manifest.yaml"), objects, options.Noun)
	default:
		data, err := MultiDocumentYAML(objects)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}
}

func writeSingleFile(path string, objects []*unstructured.Unstructured, noun string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("failed to create render output directory for %q: %w", path, err)
	}
	data, err := MultiDocumentYAML(objects)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("failed to write rendered manifests to %q: %w", path, err)
	}
	fmt.Fprintf(os.Stdout, "rendered %d %s object(s) to %s\n", len(objects), noun, path)
	return nil
}

func writeSplitFiles(outputDir string, objects []*unstructured.Unstructured, noun string) error {
	if err := os.MkdirAll(outputDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create render output directory %q: %w", outputDir, err)
	}

	for i, obj := range objects {
		path := filepath.Join(outputDir, ObjectFileName(i, obj))
		data, err := ObjectYAML(obj)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, filePerm); err != nil {
			return fmt.Errorf("failed to write rendered manifest %q: %w", path, err)
		}
	}

	fmt.Fprintf(os.Stdout, "rendered %d %s object(s) to %s\n", len(objects), noun, outputDir)
	return nil
}
