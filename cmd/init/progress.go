package initcmd

import "github.com/cloudposse/atmos/pkg/perf"

// Progress carries the startup indicator through initialization without starting
// another terminal renderer. Interactive scaffold prompts stop it before taking over.
type Progress interface {
	Update(string)
	Stop()
}

var startupProgress Progress

// SetStartupProgress shares the invocation's progress indicator with project init.
// The caller owns its lifetime and clears it after command execution.
func SetStartupProgress(progress Progress) {
	defer perf.Track(nil, "initcmd.SetStartupProgress")()
	startupProgress = progress
}

func (opts *initOptions) updateProgress(message string) {
	if opts.progress != nil {
		opts.progress.Update(message)
	}
}

func (opts *initOptions) stopProgress() {
	if opts.progress != nil {
		opts.progress.Stop()
		opts.progress = nil
	}
}
