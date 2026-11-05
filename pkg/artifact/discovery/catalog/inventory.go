package catalog

import (
	"fmt"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
)

// InventoryKind is the manifest kind of a catalog inventory: the explicit releases a maintainer
// wants listed.
const InventoryKind = "AtmosArtifactInventory"

// InventoryFile is the standard Atmos manifest envelope of an inventory.
type InventoryFile = manifest.Manifest[InventorySpec]

// InventorySpec lists the releases to resolve in the selected configured repository.
type InventorySpec struct {
	// Refs are explicit "name:version" or "name@sha256:<hex>" references, relative to the selected
	// configured repository. There is no implicit latest.
	Refs []string `yaml:"refs" json:"refs"`
}

func init() { manifest.MustRegister(InventoryKind, manifest.DefaultAPIVersion, &InventorySpec{}) }

// ParseInventory decodes and validates an inventory manifest. Every reference must name a version
// or a digest, and a reference listed twice is rejected.
func ParseInventory(raw []byte) (*InventoryFile, []artifact.Ref, error) {
	defer perf.Track(nil, "catalog.ParseInventory")()

	if int64(len(raw)) > artifact.MaxRecordSize {
		return nil, nil, fmt.Errorf("%w: the inventory is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, len(raw), artifact.MaxRecordSize)
	}
	file, err := manifest.Load[InventorySpec](InventoryKind, raw)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactCatalogInvalid, err)
	}
	if len(file.Spec.Refs) == 0 {
		return nil, nil, invalid("the inventory lists no refs")
	}
	refs := make([]artifact.Ref, 0, len(file.Spec.Refs))
	seen := make(map[string]bool, len(file.Spec.Refs))
	for _, text := range file.Spec.Refs {
		ref, err := artifact.ParseRef(text)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: inventory ref %q: %w", errUtils.ErrArtifactCatalogInvalid, text, err)
		}
		if seen[ref.String()] {
			return nil, nil, invalid("inventory ref %q is listed twice", text)
		}
		seen[ref.String()] = true
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b artifact.Ref) int { return strings.Compare(a.String(), b.String()) })
	return file, refs, nil
}
