package config

import (
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AnchorPath returns the absolute, cleaned form of p. An absolute path is only cleaned. A relative
// path resolves against the directory of the configuration file that declared it, so the result
// does not depend on the working directory or on which file was merged last.
//
// A relative path declared by a remote import cannot be anchored: its local copy lives in an
// ephemeral download directory that must never become a write root. A relative path with no known
// declaring file is likewise rejected. In both cases the caller must provide an absolute path or
// override the value in a locally declared configuration file.
func AnchorPath(origin schema.ArtifactConfigOrigin, p string) (string, error) {
	defer perf.Track(nil, "artifactconfig.AnchorPath")()

	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	switch {
	case origin.Remote:
		return "", fmt.Errorf("%w: %w: relative path %q is declared only by a remote import (%s); declare it in a local configuration file or use an absolute path", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, p, origin.File)
	case origin.Dir == "":
		return "", fmt.Errorf("%w: %w: relative path %q has no declaring configuration file to resolve against; use an absolute path", errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactUnsafePath, p)
	}
	return filepath.Join(origin.Dir, p), nil
}

// NewOrigin builds the origin of a value declared in file. Remote marks a downloaded import
// whose local path is ephemeral.
func NewOrigin(file string, remote bool) schema.ArtifactConfigOrigin {
	defer perf.Track(nil, "artifactconfig.NewOrigin")()

	if file == "" {
		return schema.ArtifactConfigOrigin{Remote: remote}
	}
	if abs, err := filepath.Abs(file); err == nil {
		file = abs
	}
	return schema.ArtifactConfigOrigin{File: file, Dir: filepath.Dir(file), Remote: remote}
}
