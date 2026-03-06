package planfile

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// BundlePlanFilename is the well-known name for the plan file within a bundle.
	BundlePlanFilename = "plan.tfplan"

	// BundleLockFilename is the well-known name for the lock file within a bundle.
	BundleLockFilename = ".terraform.lock.hcl"
)

// CreateBundle creates a tar archive containing the plan and optional lock file.
// Returns the tar bytes and the SHA256 hex string of the tar.
// The caller is responsible for setting metadata.SHA256 from the returned hash.
func CreateBundle(plan io.Reader, lockFile io.Reader) ([]byte, string, error) {
	defer perf.Track(nil, "planfile.CreateBundle")()

	if plan == nil {
		return nil, "", fmt.Errorf("%w: plan reader must not be nil", errUtils.ErrPlanfileUploadFailed)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Write plan entry.
	planData, err := io.ReadAll(plan)
	if err != nil {
		return nil, "", fmt.Errorf("%w: failed to read plan data: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	if err := writeTarEntry(tw, BundlePlanFilename, planData); err != nil {
		return nil, "", err
	}

	// Write lock file entry if present.
	if lockFile != nil {
		lockData, err := io.ReadAll(lockFile)
		if err != nil {
			return nil, "", fmt.Errorf("%w: failed to read lock file data: %w", errUtils.ErrPlanfileUploadFailed, err)
		}

		if err := writeTarEntry(tw, BundleLockFilename, lockData); err != nil {
			return nil, "", err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, "", fmt.Errorf("%w: failed to close tar archive: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	tarBytes := buf.Bytes()

	// Compute SHA256 of the complete tar.
	hash := sha256.Sum256(tarBytes)
	sha256Hex := hex.EncodeToString(hash[:])

	return tarBytes, sha256Hex, nil
}

// ExtractBundle extracts plan and lock file from a tar archive.
// Returns the plan data, lock file data (nil if not present), and error.
func ExtractBundle(data io.Reader) (plan []byte, lockFile []byte, err error) {
	defer perf.Track(nil, "planfile.ExtractBundle")()

	tr := tar.NewReader(data)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%w: failed to read tar entry: %w", errUtils.ErrPlanfileDownloadFailed, err)
		}

		switch header.Name {
		case BundlePlanFilename:
			plan, err = io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: failed to read plan from bundle: %w", errUtils.ErrPlanfileDownloadFailed, err)
			}
		case BundleLockFilename:
			lockFile, err = io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("%w: failed to read lock file from bundle: %w", errUtils.ErrPlanfileDownloadFailed, err)
			}
		}
	}

	if plan == nil {
		return nil, nil, fmt.Errorf("%w: %s not found in bundle", errUtils.ErrPlanfileDownloadFailed, BundlePlanFilename)
	}

	return plan, lockFile, nil
}

// writeTarEntry writes a single file entry to the tar writer.
func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(data)),
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("%w: failed to write tar header for %s: %w", errUtils.ErrPlanfileUploadFailed, name, err)
	}

	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("%w: failed to write tar data for %s: %w", errUtils.ErrPlanfileUploadFailed, name, err)
	}

	return nil
}
