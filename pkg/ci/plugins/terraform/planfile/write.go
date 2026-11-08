package planfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Mode of a downloaded plan or lock file.
	downloadedFileMode os.FileMode = 0o644
	// Owner-only mode of a downloaded plan record.
	privateFileMode os.FileMode = 0o600
)

// BindingPath returns where the plan record of a plan file is kept on disk.
func BindingPath(planfilePath string) string {
	defer perf.Track(nil, "planfile.BindingPath")()

	return planfilePath + bindingSuffix
}

// OpenBinding opens the plan record beside a plan file for upload. It returns a nil closer when there
// is none, which is the normal case for components that do not keep one. A record that exists but cannot
// be opened is an error: uploading the plan without it would leave a fresh runner unable to apply it.
// The caller closes the returned file.
func OpenBinding(planfilePath string) (FileEntry, io.Closer, error) {
	defer perf.Track(nil, "planfile.OpenBinding")()

	path := BindingPath(planfilePath)
	file, err := os.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return FileEntry{}, nil, nil
	case err != nil:
		return FileEntry{}, nil, fmt.Errorf("%w: failed to open plan record %s: %w", errUtils.ErrPlanfileUploadFailed, path, err)
	}
	return FileEntry{Name: BindingFilename, Data: file, Size: -1}, file, nil
}

// WritePlanfileResults writes downloaded planfile results to disk.
// It maps PlanFilename to planfilePath, BindingFilename to the plan record beside it, and
// LockFilename to the same directory.
// Parent directories are created as needed. Unknown filenames are skipped.
func WritePlanfileResults(results []FileResult, planfilePath string) error {
	defer perf.Track(nil, "planfile.WritePlanfileResults")()

	componentDir := filepath.Dir(planfilePath)

	for _, r := range results {
		var destPath string
		switch r.Name {
		case PlanFilename:
			destPath = planfilePath
		case BindingFilename:
			destPath = BindingPath(planfilePath)
		case LockFilename:
			destPath = filepath.Join(componentDir, LockFilename)
		default:
			continue
		}

		if err := writeFileResult(r, destPath, fileMode(r.Name)); err != nil {
			return err
		}
	}

	return nil
}

// fileMode keeps the plan record owner-only: it can carry generated credentials or extra-argument
// environment values, which other users of a shared runner must not read.
func fileMode(name string) os.FileMode {
	if name == BindingFilename {
		return privateFileMode
	}
	return downloadedFileMode
}

// WritePlanfileResultsForVerification writes downloaded planfile results for plan verification.
// The plan file is written to storedPlanPath (with stored prefix), while the lock file
// is written relative to canonicalPlanPath (the canonical location where terraform expects it).
// This separation allows terraform to generate a fresh plan at the canonical path for comparison.
func WritePlanfileResultsForVerification(results []FileResult, storedPlanPath, canonicalPlanPath string) error {
	defer perf.Track(nil, "planfile.WritePlanfileResultsForVerification")()

	canonicalDir := filepath.Dir(canonicalPlanPath)

	for _, r := range results {
		var destPath string
		switch r.Name {
		case PlanFilename:
			destPath = storedPlanPath
		case LockFilename:
			destPath = filepath.Join(canonicalDir, LockFilename)
			// Verification must use the landed checkout's dependency selections,
			// not replace them with an older PR's provider lockfile.
			if _, err := os.Stat(destPath); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				return err
			}
		default:
			continue
		}

		if err := writeFileResult(r, destPath, fileMode(r.Name)); err != nil {
			return err
		}
	}

	return nil
}

// writeFileResult writes a single file result to the given destination path with mode. Replacing
// an existing file also resets its mode, so a looser earlier copy cannot keep a record readable.
func writeFileResult(r FileResult, destPath string, mode os.FileMode) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("%w: failed to create directory for %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, err)
	}

	fileData, err := io.ReadAll(r.Data)
	if err != nil {
		return fmt.Errorf("%w: failed to read %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, err)
	}
	file, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("%w: failed to write %s to %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, destPath, err)
	}
	_, writeErr := file.Write(fileData)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr, os.Chmod(destPath, mode)); err != nil {
		return fmt.Errorf("%w: failed to write %s to %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, destPath, err)
	}

	return nil
}
