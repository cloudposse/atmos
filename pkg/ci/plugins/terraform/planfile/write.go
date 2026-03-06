package planfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WritePlanfileResults writes downloaded planfile results to disk.
// It maps PlanFilename to planfilePath and LockFilename to the same directory.
// Parent directories are created as needed. Unknown filenames are skipped.
func WritePlanfileResults(results []FileResult, planfilePath string) error {
	defer perf.Track(nil, "planfile.WritePlanfileResults")()

	componentDir := filepath.Dir(planfilePath)

	for _, r := range results {
		var destPath string
		switch r.Name {
		case PlanFilename:
			destPath = planfilePath
		case LockFilename:
			destPath = filepath.Join(componentDir, LockFilename)
		default:
			continue
		}

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
	}

	return nil
}
