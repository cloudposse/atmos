package output

import (
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// wrapDescribeError wraps an error from DescribeComponent using the shared helper.
func wrapDescribeError(component, stack string, err error) error {
	return errUtils.WrapComponentDescribeError(component, stack, err, "component")
}

// terraformOutputsCache caches terraform outputs by stack-component key.
var terraformOutputsCache = sync.Map{}

// ResetOutputsCache clears the terraform outputs cache.
// This is exported for use in tests to ensure cache isolation between test functions.
func ResetOutputsCache() {
	defer perf.Track(nil, "output.ResetOutputsCache")()

	terraformOutputsCache.Range(func(key, _ any) bool {
		terraformOutputsCache.Delete(key)
		return true
	})
}

// stackComponentKey builds an unambiguous cache key from stack and component names.
// A null byte separator prevents collisions when either name contains hyphens
// (e.g. stack "us-east-1-dev" + component "vpc" must not collide with
// stack "us-east-1" + component "dev-vpc").
func stackComponentKey(stack, component string) string {
	return stack + "\x00" + component
}

// quietModeWriter captures output during quiet mode operations.
// On success, output is discarded. On failure, captured stderr is included in errors.
type quietModeWriter struct {
	buffer *strings.Builder
}

func newQuietModeWriter() *quietModeWriter {
	return &quietModeWriter{buffer: &strings.Builder{}}
}

// Write implements io.Writer interface.
func (w *quietModeWriter) Write(p []byte) (n int, err error) {
	defer perf.Track(nil, "output.quietModeWriter.Write")()

	return w.buffer.Write(p)
}

// String returns the captured output.
func (w *quietModeWriter) String() string {
	defer perf.Track(nil, "output.quietModeWriter.String")()

	return w.buffer.String()
}

// wrapErrorWithStderr wraps an error with captured stderr output if available.
// Used in quiet mode to include terraform output in error messages on failure.
func wrapErrorWithStderr(err error, capture *quietModeWriter) error {
	if capture == nil || capture.String() == "" {
		return err
	}
	return errUtils.Build(errUtils.ErrTerraformOutputFailed).
		WithCause(err).
		WithExplanation(strings.TrimSpace(capture.String())).
		Err()
}

// checkOutputsCache checks if terraform outputs are already cached for the given stack/component.
func checkOutputsCache(stackSlug, component, stack string) map[string]any {
	cachedOutputs, found := terraformOutputsCache.Load(stackSlug)
	if found && cachedOutputs != nil {
		log.Debug("Cache hit for terraform outputs", "stack", stack, "component", component)
		return cachedOutputs.(map[string]any)
	}
	return nil
}

// startSpinnerOrLog starts a spinner in normal mode or logs in debug mode, returns a stop function.
func startSpinnerOrLog(atmosConfig *schema.AtmosConfiguration, message, _, _ string) func() {
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		log.Debug(message)
		return func() {}
	}
	p := NewSpinner(message)
	spinnerDone := make(chan struct{})
	RunSpinner(p, spinnerDone, message)
	return func() { StopSpinner(p, spinnerDone) }
}
