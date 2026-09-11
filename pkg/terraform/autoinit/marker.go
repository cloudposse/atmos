package autoinit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MarkerFileName is the name of the init marker file, written inside the Terraform data
// directory.
const MarkerFileName = "atmos-init.json"

// MarkerSchemaVersion is the current on-disk schema version for Marker. Bumping it forces every
// existing marker to be treated as stale (ReasonSchemaVersion) the next time Decide runs.
const MarkerSchemaVersion = 1

// markerDirPerm and markerFilePerm are the permissions used when creating the marker's parent
// directory and the marker file itself.
const (
	markerDirPerm  = 0o755
	markerFilePerm = 0o644
)

// Marker is the small JSON record Atmos writes after a successful `terraform init`, capturing the
// fingerprint that was true at that moment so a later run can decide whether init is still
// unnecessary.
type Marker struct {
	// SchemaVersion is the on-disk schema version of this marker; compared against
	// MarkerSchemaVersion by Decide.
	SchemaVersion int `json:"schema_version"`
	// Fingerprint is the Fingerprint.Hash recorded at init time.
	Fingerprint string `json:"fingerprint"`
	// BackendFingerprint is the Fingerprint.BackendHash recorded at init time.
	BackendFingerprint string `json:"backend_fingerprint"`
	// InitArgs are the extra arguments (e.g. "-upgrade", "-reconfigure") the recorded init ran
	// with.
	InitArgs []string `json:"init_args"`
	// AtmosVersion is the Atmos version that performed the init.
	AtmosVersion string `json:"atmos_version"`
	// Binary is the terraform/tofu executable that performed the init.
	Binary string `json:"binary"`
	// Timestamp is when the recorded init completed, in UTC.
	Timestamp time.Time `json:"timestamp"`
}

// ReadMarker reads and parses the marker at path. A missing file or malformed JSON is not treated
// as an error -- both return (nil, nil), since either simply means "no usable marker yet". Any
// other I/O failure (e.g. a permissions error, or path referring to a directory) is a genuine
// failure and returns an ErrInitMarker-wrapped error.
func ReadMarker(path string) (*Marker, error) {
	defer perf.Track(nil, "autoinit.ReadMarker")()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil //nolint:nilnil // absence is a valid, non-error outcome; see doc comment.
		}
		return nil, fmt.Errorf("%w: reading %s: %w", errUtils.ErrInitMarker, path, err)
	}

	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		log.Debug("autoinit: init marker is malformed, ignoring", "path", path, "error", err)
		return nil, nil //nolint:nilnil // malformed marker is treated as "no marker", not an error.
	}
	return &m, nil
}

// WriteMarker writes m to path as JSON, creating parent directories as needed and writing
// atomically so a concurrent reader never observes a partially written marker.
func WriteMarker(path string, m *Marker) error {
	defer perf.Track(nil, "autoinit.WriteMarker")()

	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("%w: encoding %s: %w", errUtils.ErrInitMarker, path, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, markerDirPerm); err != nil {
		return fmt.Errorf("%w: creating %s: %w", errUtils.ErrInitMarker, dir, err)
	}

	fs := filesystem.NewOSFileSystem()
	if err := fs.WriteFileAtomic(path, data, markerFilePerm); err != nil {
		return fmt.Errorf("%w: writing %s: %w", errUtils.ErrInitMarker, path, err)
	}
	return nil
}

// Record recomputes the fingerprint from in (the lock file, and anything else init may have
// changed, is re-read post-init) and writes the resulting marker to MarkerPath(dataDir).
func Record(in *Inputs, initArgs []string, atmosVersion string) error {
	defer perf.Track(nil, "autoinit.Record")()

	fp, err := Compute(in)
	if err != nil {
		return err
	}

	m := &Marker{
		SchemaVersion:      MarkerSchemaVersion,
		Fingerprint:        fp.Hash,
		BackendFingerprint: fp.BackendHash,
		InitArgs:           initArgs,
		AtmosVersion:       atmosVersion,
		Binary:             in.Binary,
		Timestamp:          time.Now().UTC(),
	}

	return WriteMarker(MarkerPath(in.effectiveDataDir()), m)
}
