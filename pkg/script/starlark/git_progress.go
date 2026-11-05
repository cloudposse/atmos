package starlark

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/progress"
	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/git/sandbox"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// openSandbox finishes its preparation renderer before the callback starts a test UI.
// Copy progress stays scoped and static when invoked inside a capture or parallel task.
//
//nolint:gocritic // Copy options so the progress callback stays local to preparation.
func (s *session) openSandbox(ctx context.Context, t *starlark.Thread, opts sandbox.Options) (*sandbox.Sandbox, error) {
	term := terminal.New()
	live := s.ownsTerminal(t) && !ci.IsCI() && term.IsTTY(terminal.Stderr) && term.IsTTY(terminal.Stdout)
	activity := spinner.New("Preparing isolated checkout")
	const progressWidth = 18
	bar := ui.NewProgress(progress.WithWidth(progressWidth), progress.WithoutPercentage())
	if live {
		activity.SetInterruptHandler(s.interrupt.request)
		activity.Start()
		defer func() {
			activity.Stop()
			if s.interrupt.interrupted() {
				fmt.Fprintln(s.writer(t, stderrStream), "Checks canceled.")
			}
		}()
	}
	lastPhase := ""
	opts.OnProgress = func(update sandbox.Progress) {
		message := update.Phase
		if update.Total > 0 {
			message = fmt.Sprintf("%s · %d/%d files", message, update.Completed, update.Total)
			if live {
				message = fmt.Sprintf("%s %s · %d/%d files", update.Phase, bar.ViewAs(float64(update.Completed)/float64(update.Total)), update.Completed, update.Total)
			}
		}
		if live {
			activity.Update(message)
		} else if update.Phase != lastPhase {
			fmt.Fprintln(s.writer(t, stderrStream), message)
		}
		lastPhase = update.Phase
	}
	return sandbox.Open(ctx, opts)
}
