package planfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// BindingPath returns where the plan record of a plan file is kept on disk.
func BindingPath(planfilePath string) string {
	defer perf.Track(nil, "planfile.BindingPath")()

	return planfilePath + bindingSuffix
}

// OpenBinding opens the plan record beside a plan file for upload. It reports false when there is
// none, which is the normal case for components that do not keep one. The caller closes the returned file.
func OpenBinding(planfilePath string) (FileEntry, io.Closer, bool) {
	defer perf.Track(nil, "planfile.OpenBinding")()

	file, err := os.Open(BindingPath(planfilePath))
	if err != nil {
		return FileEntry{}, nil, false
	}
	return FileEntry{Name: BindingFilename, Data: file, Size: -1}, file, true
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

		if err := writeFileResult(r, destPath); err != nil {
			return err
		}
	}

	return nil
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

		if err := writeFileResult(r, destPath); err != nil {
			return err
		}
	}

	return nil
}

// writeFileResult writes a single file result to the given destination path.
func writeFileResult(r FileResult, destPath string) error {
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("%w: failed to create directory for %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, err)
	}

	fileData, err := io.ReadAll(r.Data)
	if err != nil {
		return fmt.Errorf("%w: failed to read %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, err)
	}
	if err := os.WriteFile(destPath, fileData, 0o644); err != nil {
		return fmt.Errorf("%w: failed to write %s to %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, destPath, err)
	}

	return nil
}
