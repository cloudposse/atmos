package produce

import (
	"context"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ProducerMetadata is name, version, and descriptive metadata a producer derives from its source
// (for example a scaffold manifest). Generic file packages need none of it.
type ProducerMetadata struct {
	// Name is the producer-supplied artifact name.
	Name string

	// Version is the producer-supplied version.
	Version string

	// Description is the producer-supplied description.
	Description string

	// Labels are producer-supplied default labels; explicit labels override them.
	Labels map[string]string

	// KindData is optional kind-specific data stored in the Record.
	KindData artifact.KindData
}

// MetadataSource reads producer metadata for a source. A source without metadata returns the zero
// value and no error.
type MetadataSource func(ctx context.Context, source string) (ProducerMetadata, error)

// ResolveIdentity applies the name and version precedence rules. Name precedence is the explicit
// value, then producer metadata, then the source basename. Version precedence is the explicit
// value, then producer metadata; otherwise it fails because there is no generated version and no
// implicit latest. Backend-invalid values are rejected rather than normalized.
func ResolveIdentity(explicitName, explicitVersion string, meta *ProducerMetadata, source string) (string, string, error) {
	defer perf.Track(nil, "produce.ResolveIdentity")()

	name := firstNonEmpty(explicitName, meta.Name)
	if name == "" {
		base, err := sourceBasename(source)
		if err != nil {
			return "", "", err
		}
		name = base
		if err := artifact.ValidateName(name); err != nil {
			return "", "", fmt.Errorf("%w: the source basename %q is not a valid artifact name; pass an explicit name", err, name)
		}
	}
	if err := artifact.ValidateName(name); err != nil {
		return "", "", err
	}
	version := firstNonEmpty(explicitVersion, meta.Version)
	if version == "" {
		return "", "", fmt.Errorf("%w: a version is required (there is no generated version and no implicit latest)", errUtils.ErrArtifactRefInvalid)
	}
	if err := artifact.ValidateVersion(version); err != nil {
		return "", "", err
	}
	return name, version, nil
}

func sourceBasename(source string) (string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("%w: source %q: %w", errUtils.ErrArtifactPackagingInvalid, source, err)
	}
	return filepath.Base(abs), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// mergeLabels returns producer labels overlaid by explicit labels, or nil when there are none.
func mergeLabels(producer, explicit map[string]string) map[string]string {
	if len(producer) == 0 && len(explicit) == 0 {
		return nil
	}
	merged := make(map[string]string, len(producer)+len(explicit))
	for key, value := range producer {
		merged[key] = value
	}
	for key, value := range explicit {
		merged[key] = value
	}
	return merged
}
