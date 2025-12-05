package toolchain

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// DuExec calculates and displays the total disk space used by installed tools.
func DuExec() error {
	defer perf.Track(nil, "toolchain.DuExec")()

	// Get the install path where tools are stored.
	installPath := GetInstallPath()

	// Calculate total size using existing function from install.go.
	totalSize, err := calculateDirectorySize(installPath)
	if err != nil {
		// If the directory doesn't exist, report 0 usage.
		if os.IsNotExist(err) {
			_ = ui.Info("No tools installed (0 B)")
			return nil
		}
		return fmt.Errorf("failed to calculate disk usage: %w", err)
	}

	// Format size in human-readable format using existing function from install.go.
	humanSize := formatBytes(totalSize)

	// Display using ui.Info.
	_ = ui.Infof("Total disk space used by installed tools: %s", humanSize)

	return nil
}

// calculateDirectorySize recursively calculates the total size of a directory in bytes.
func calculateDirectorySize(dirPath string) (int64, error) {
	defer perf.Track(nil, "toolchain.calculateDirectorySize")()

	var size int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files/directories we can't access.
			//nolint:nilerr // Intentionally ignore inaccessible paths.
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// formatBytes formats a byte count into a human-readable string.
// Uses lowercase suffix style (e.g., "120mb", "1.5gb") for consistency with user's example.
func formatBytes(bytes int64) string {
	defer perf.Track(nil, "toolchain.formatBytes")()

	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fgb", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%dmb", bytes/MB)
	case bytes >= KB:
		return fmt.Sprintf("%dkb", bytes/KB)
	default:
		return fmt.Sprintf("%db", bytes)
	}
}
