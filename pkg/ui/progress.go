package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/spinner/fps"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// NewProgress creates a progress bar using the active theme and Atmos color profile.
// Options can customize the bar without repeating terminal detection at call sites.
func NewProgress(opts ...progress.Option) progress.Model {
	defer perf.Track(nil, "ui.NewProgress")()

	styles := theme.GetCurrentStyles()
	defaults := []progress.Option{
		progress.WithGradient(theme.GetSpinnerColor(), theme.GetSuccessColor()),
		progress.WithColorProfile(GetColorProfile()),
		func(bar *progress.Model) {
			bar.EmptyColor = fmt.Sprint(styles.Muted.GetForeground())
			bar.PercentageStyle = styles.Body
		},
	}
	return progress.New(append(defaults, opts...)...)
}

// NewSpinner creates the shared Dot animation with theme colors and the configured frame rate.
func NewSpinner() spinner.Model {
	defer perf.Track(nil, "ui.NewSpinner")()

	s := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.GetCurrentStyles().Spinner))
	fps.Apply(&s)
	return s
}
