package output

import (
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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

// cachedOutputResult is the outcome of a cache-hit output resolution (see
// resolveOutputFromCache). A nil *cachedOutputResult means the cache held no
// entry for the requested stackSlug; callers must fall through to a real
// fetch in that case.
type cachedOutputResult struct {
	value  any
	exists bool
	err    error
}

// resolveOutputFromCache resolves a single output from the cache, if present,
// emitting the same visible success/failure notification (outputLookupSucceeded/
// outputLookupFailed) a real fetch would via GetOutput/GetOutputWithOptions.
//
// Without this, a component's first output lookup (a cache miss, triggering a
// real fetch) is visible, but every subsequent lookup for a DIFFERENT output
// key on the SAME already-cached component+stack was silently invisible: the
// cache is keyed per component+stack (all outputs cached together on first
// fetch, by design, for performance), but only "Cache hit for terraform
// output" was logged -- at Debug level only, so it never appeared outside
// debug/trace logging. A test.vars block with nine !terraform.output lookups
// across two components would then show only two "Fetching ..." messages.
func resolveOutputFromCache(atmosConfig *schema.AtmosConfiguration, stackSlug, component, stack, output string) *cachedOutputResult {
	cachedOutputs, found := terraformOutputsCache.Load(stackSlug)
	if !found || cachedOutputs == nil {
		return nil
	}

	log.Debug("Cache hit for terraform output", "stack", stack, "component", component, "output", output)
	message := fmt.Sprintf("Fetching %s output from %s in %s", output, component, stack)

	value, exists, err := getOutputVariable(atmosConfig, component, stack, cachedOutputs.(map[string]any), output)
	if err != nil {
		outputLookupFailed(message)
		return &cachedOutputResult{err: err}
	}
	outputLookupSucceeded(message)
	return &cachedOutputResult{value: value, exists: exists}
}

// startSpinnerOrLog starts a spinner in normal mode or logs in debug mode.
func startSpinnerOrLog(atmosConfig *schema.AtmosConfiguration, message, _, _ string) func() {
	if strings.EqualFold(atmosConfig.Logs.Level, string(u.LogLevelTrace)) ||
		strings.EqualFold(atmosConfig.Logs.Level, string(u.LogLevelDebug)) {
		log.Debug(message)
		return func() {}
	}
	if spinnersSuppressed() {
		return func() {}
	}
	p := NewSpinner(message)
	spinnerDone := make(chan struct{})
	RunSpinner(p, spinnerDone, message)
	return func() { StopSpinner(p, spinnerDone) }
}

// clearSpinnerLine clears transient lookup output unless concurrent execution suppressed it.
func clearSpinnerLine() {
	writeVisibleOutput(ui.ClearLine)
}

// writeVisibleOutput renders transient lookup UI unless concurrent execution suppressed it.
func writeVisibleOutput(write func()) {
	spinnerSuppression.Lock()
	defer spinnerSuppression.Unlock()

	if spinnerSuppression.count == 0 {
		write()
	}
}

func outputLookupSucceeded(message string) {
	writeVisibleOutput(func() {
		ui.ClearLine()
		ui.Success(message)
	})
}

func outputLookupFailed(message string) {
	writeVisibleOutput(func() {
		ui.ClearLine()
		ui.Error(message)
	})
}

func outputLookupVisible() bool {
	return !spinnersSuppressed()
}
