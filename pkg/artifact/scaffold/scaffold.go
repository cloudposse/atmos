// Package scaffold validates scaffold sources for publication.
//
// Validation loads the scaffold manifest (scaffold.yaml at the source root) through the same
// loader that generation and `atmos scaffold validate` use, so a published scaffold is exactly as
// valid as a generated one. It parses and validates configuration only: it never renders templates,
// writes generated output, or runs scaffold hooks.
//
// The package sits above pkg/artifact/produce in the dependency graph because the scaffold loader
// pulls in the generator, hook, and workflow packages. The scaffold command registers the validator
// with [Register] when the binary starts.
package scaffold

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// Validator validates scaffold sources for the scaffold producer.
type Validator struct{}

var _ produce.ScaffoldValidator = Validator{}

// Register installs the validator as the process-wide scaffold validator of the producers.
func Register() {
	defer perf.Track(nil, "scaffold.Register")()

	produce.RegisterScaffoldValidator(Validator{})
}

// ValidateScaffold validates the scaffold at dir and returns the name, version, and description its
// manifest declares. The manifest must be scaffold.yaml at the root, because generation looks only
// there.
func (Validator) ValidateScaffold(ctx context.Context, dir string) (produce.ProducerMetadata, error) {
	defer perf.Track(nil, "scaffold.Validator.ValidateScaffold")()

	if err := ctx.Err(); err != nil {
		return produce.ProducerMetadata{}, err
	}
	manifestPath := filepath.Join(dir, config.ScaffoldConfigFileName)
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return produce.ProducerMetadata{}, fmt.Errorf("%w: %s does not contain %s at its root", errUtils.ErrArtifactScaffoldInvalid, dir, config.ScaffoldConfigFileName)
		}
		return produce.ProducerMetadata{}, fmt.Errorf("%w: read %s: %w", errUtils.ErrArtifactScaffoldInvalid, manifestPath, err)
	}
	manifest, err := config.LoadScaffoldConfigFromContent(string(content), config.WithSourceDir(dir))
	if err != nil {
		return produce.ProducerMetadata{}, fmt.Errorf("%w: %s: %w", errUtils.ErrArtifactScaffoldInvalid, manifestPath, err)
	}
	return produce.ProducerMetadata{
		Name:        manifest.Metadata.Name,
		Version:     manifest.Metadata.Version,
		Description: manifest.Metadata.Description,
	}, nil
}
