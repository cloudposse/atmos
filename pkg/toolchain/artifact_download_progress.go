package toolchain

import (
	"fmt"
	"math"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/x/ansi"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	artifactCompletePercent  = 100
	artifactProgressInterval = 100 * time.Millisecond
	artifactLogInterval      = 5 * time.Second
	artifactBarWidth         = 24
	artifactMinBarWidth      = 6
	// Reserve the spinner (including its trailing space), two separators, and the final column.
	artifactProgressMargin = 5
)

type artifactDownloadOptions struct {
	idleTimeout time.Duration
	// complete is true only after the response body has been copied successfully.
	progress func(downloaded, total int64, complete bool)
}

func formatArtifactDownloadProgress(name string, downloaded, total int64) string {
	if total <= 0 {
		return fmt.Sprintf("Downloading %s (%s received)", name, formatBytes(downloaded))
	}
	percent := math.Min(artifactCompletePercent, float64(downloaded)/float64(total)*artifactCompletePercent)
	message := fmt.Sprintf("Downloading %s (%s / %s, %.0f%%)", name, formatBytes(downloaded), formatBytes(total), percent)
	if !term.IsTTYSupportForStdout() {
		return message
	}
	width := terminal.New().Width(terminal.Stderr) - ansi.StringWidth(message) - artifactProgressMargin
	if width < artifactMinBarWidth {
		return message
	}
	bar := ui.NewProgress(progress.WithWidth(min(artifactBarWidth, width)), progress.WithoutPercentage())
	// Format only the label; the progress component already contains rendered ANSI.
	return ui.FormatInline(message) + " " + bar.ViewAs(percent/artifactCompletePercent)
}

// newArtifactProgressReporter limits terminal redraws and keeps non-TTY logs
// sparse. Progress accounting and the inactivity timer still run on every read.
func newArtifactProgressReporter(name string, update func(string)) func(int64, int64, bool) {
	interval := artifactProgressInterval
	if !term.IsTTYSupportForStdout() {
		interval = artifactLogInterval
	}
	var lastUpdate time.Time
	var lastMessage string
	return func(downloaded, total int64, complete bool) {
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
