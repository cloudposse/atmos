package errors

import "sync"

var (
	failureReporterMu sync.RWMutex
	failureReporter   func(error)
)

// SetFailureReporter installs the function that reports a command's failure to the environment, such as
// CI annotations, before CheckErrorPrintAndExit ends the process. Failures returned to main are reported
// there; the commands that exit the process directly would otherwise never be reported. It returns a
// function that restores the previous reporter.
func SetFailureReporter(report func(error)) func() {
	failureReporterMu.Lock()
	defer failureReporterMu.Unlock()

	previous := failureReporter
	failureReporter = report
	return func() {
		failureReporterMu.Lock()
		defer failureReporterMu.Unlock()
		failureReporter = previous
	}
}

// reportFailure hands err to the installed reporter, if any.
func reportFailure(err error) {
	failureReporterMu.RLock()
	report := failureReporter
	failureReporterMu.RUnlock()

	if report != nil {
		report(err)
	}
}
