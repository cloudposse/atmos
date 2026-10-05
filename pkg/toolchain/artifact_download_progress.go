package toolchain

import (
	"fmt"
	"math"
	"time"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

const (
	artifactCompletePercent  = 100
	artifactProgressInterval = 100 * time.Millisecond
	artifactLogInterval      = 5 * time.Second
)

type artifactDownloadOptions struct {
	idleTimeout time.Duration
	progress    func(downloaded, total int64)
}

func formatArtifactDownloadProgress(name string, downloaded, total int64) string {
	if total <= 0 {
		return fmt.Sprintf("Downloading %s (%s received)", name, formatBytes(downloaded))
	}
	percent := math.Min(artifactCompletePercent, float64(downloaded)/float64(total)*artifactCompletePercent)
	return fmt.Sprintf("Downloading %s (%s / %s, %.0f%%)", name, formatBytes(downloaded), formatBytes(total), percent)
}

// newArtifactProgressReporter limits terminal redraws and keeps non-TTY logs
// sparse. Progress accounting and the inactivity timer still run on every read.
func newArtifactProgressReporter(name string, update func(string)) func(int64, int64) {
	interval := artifactProgressInterval
	if !term.IsTTYSupportForStdout() {
		interval = artifactLogInterval
	}
	var lastUpdate time.Time
	var lastMessage string
	return func(downloaded, total int64) {
		complete := total > 0 && downloaded >= total
		if !lastUpdate.IsZero() && time.Since(lastUpdate) < interval && !complete {
			return
		}
		message := formatArtifactDownloadProgress(name, downloaded, total)
		if message != lastMessage {
			update(message)
			lastUpdate = time.Now()
			lastMessage = message
		}
	}
}
