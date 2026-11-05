package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// fileMode is the permission of a written catalog snapshot.
const fileMode = 0o644

// readBounded reads a local manifest file within the catalog size bound.
func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, artifact.MaxRecordSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > artifact.MaxRecordSize {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", errUtils.ErrArtifactTooLarge, path, artifact.MaxRecordSize)
	}
	return raw, nil
}

// ReadInventoryFile reads and validates an AtmosArtifactInventory manifest from a local file.
func ReadInventoryFile(path string) ([]artifact.Ref, error) {
	defer perf.Track(nil, "catalog.ReadInventoryFile")()

	raw, err := readBounded(path)
	if err != nil {
		return nil, err
	}
	_, refs, err := ParseInventory(raw)
	return refs, err
}

// ReadFile reads and validates an AtmosArtifactCatalog manifest from a local file.
func ReadFile(path string) (*File, error) {
	defer perf.Track(nil, "catalog.ReadFile")()

	raw, err := readBounded(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

// WriteFile atomically writes a catalog snapshot. The format follows the file extension: YAML for
// .yaml and .yml, JSON otherwise.
func WriteFile(path string, file *File) error {
	defer perf.Track(nil, "catalog.WriteFile")()

	Normalize(file)
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if extension := strings.ToLower(filepath.Ext(path)); extension == ".yaml" || extension == ".yml" {
		if raw, err = sigsyaml.JSONToYAML(raw); err != nil {
			return err
		}
	} else {
		raw = append(raw, '\n')
	}
	if _, err := Parse(raw); err != nil {
		return err
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(path, raw, fileMode)
}
