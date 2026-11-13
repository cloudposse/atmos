package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// signalsFilePermissions is the mode of the ignore signals file.
const signalsFilePermissions = 0o644

// SignalsFileName is the file that an ignore policy writes its signals to, in the module directory.
const SignalsFileName = "error-signals.json"

// patternNoise matches everything that is not part of a readable error message. Terragrunt cleans
// error text this way before matching, so a pattern written for it matches here too.
var patternNoise = regexp.MustCompile(`[^a-zA-Z0-9./'"():=\- ]+`)

// Reporter tells the user what error handling decided. Implementations print on the UI channel.
type Reporter interface {
	// Retrying reports that the command failed with a retryable error and will run again.
	Retrying(policy string, attempt, maxAttempts int, sleep time.Duration)
	// Ignored reports that a failure was turned into a success.
	Ignored(policy, message string)
	// SignalsWritten reports where an ignore policy wrote its signals.
	SignalsWritten(path string)
}

// Recoverer applies a provider's errors policy to a failing command: an ignore policy turns the
// failure into a success, and a retry policy runs the command again after a pause. Ignore
// policies are checked before retry policies, each in declaration order, and the first match
// decides, like Terragrunt's errors block.
type Recoverer struct {
	policy      *schema.ComponentErrors
	reporter    Reporter
	signalsFile string
	sleep       func(ctx context.Context, duration time.Duration) error
}

// RecovererOption configures a Recoverer.
type RecovererOption func(*Recoverer)

// WithReporter sets where decisions are reported.
func WithReporter(reporter Reporter) RecovererOption {
	defer perf.Track(nil, "execution.WithReporter")()

	return func(r *Recoverer) { r.reporter = reporter }
}

// WithSignalsFile sets the file an ignore policy writes its signals to. Without it no signals are written.
func WithSignalsFile(path string) RecovererOption {
	defer perf.Track(nil, "execution.WithSignalsFile")()

	return func(r *Recoverer) { r.signalsFile = path }
}

// WithSleep replaces the pause between attempts, so tests do not wait.
func WithSleep(sleep func(ctx context.Context, duration time.Duration) error) RecovererOption {
	defer perf.Track(nil, "execution.WithSleep")()

	return func(r *Recoverer) { r.sleep = sleep }
}

// NewRecoverer builds a Recoverer for a policy. A nil or empty policy never recovers.
func NewRecoverer(policy *schema.ComponentErrors, options ...RecovererOption) *Recoverer {
	defer perf.Track(nil, "execution.NewRecoverer")()

	recoverer := &Recoverer{policy: policy, sleep: sleepContext}
	for _, option := range options {
		option(recoverer)
	}
	return recoverer
}

// Attempt runs the command once. It returns the error text the policies match against, usually the
// command's standard error, and the command's error.
type Attempt func(ctx context.Context) (text string, err error)

// Run runs the attempt until it succeeds, fails with an error no policy covers, or exhausts a
// retry policy. It returns ignored = true when an ignore policy swallowed the failure.
func (r *Recoverer) Run(ctx context.Context, attempt Attempt) (ignored bool, err error) {
	defer perf.Track(nil, "execution.Recoverer.Run")()

	for current := 1; ; current++ {
		text, runErr := attempt(ctx)
		if runErr == nil {
			return false, nil
		}
		if r == nil || r.policy == nil || ctx.Err() != nil {
			return false, runErr
		}
		if strings.TrimSpace(text) == "" {
			text = runErr.Error()
		}
		retry, ignored, err := r.decide(ctx, cleanErrorText(text), runErr, current)
		if !retry {
			return ignored, err
		}
	}
}

// decide applies the policies to one failure. It reports whether the command should run again;
// when it should not, ignored and err describe the final outcome.
func (r *Recoverer) decide(ctx context.Context, text string, runErr error, current int) (retry, ignored bool, err error) {
	if policy := r.matchingIgnore(text); policy != nil {
		r.ignore(policy)
		return false, true, nil
	}
	policy := r.matchingRetry(text)
	if policy == nil {
		return false, false, runErr
	}
	if current >= policy.MaxAttempts {
		return false, false, fmt.Errorf("%w (%d) for policy %q: %w", errUtils.ErrComponentMaxAttempts, policy.MaxAttempts, policy.Name, runErr)
	}
	if r.reporter != nil {
		r.reporter.Retrying(policy.Name, current, policy.MaxAttempts, policy.SleepInterval)
	}
	if err := r.sleep(ctx, policy.SleepInterval); err != nil {
		return false, false, err
	}
	return true, false, nil
}

func (r *Recoverer) matchingIgnore(text string) *schema.ComponentIgnorePolicy {
	for i := range r.policy.Ignore {
		if matchesPatterns(text, r.policy.Ignore[i].Patterns) {
			return &r.policy.Ignore[i]
		}
	}
	return nil
}

func (r *Recoverer) matchingRetry(text string) *schema.ComponentRetryPolicy {
	for i := range r.policy.Retry {
		if matchesPatterns(text, r.policy.Retry[i].Patterns) {
			return &r.policy.Retry[i]
		}
	}
	return nil
}

// ignore reports the decision and writes the signals. A signals file that cannot be written is
// reported but never turns the ignored failure back into an error.
func (r *Recoverer) ignore(policy *schema.ComponentIgnorePolicy) {
	if r.reporter != nil {
		r.reporter.Ignored(policy.Name, policy.Message)
	}
	if len(policy.Signals) == 0 || r.signalsFile == "" {
		return
	}
	data, err := json.MarshalIndent(policy.Signals, "", "  ")
	if err == nil {
		err = filesystem.NewOSFileSystem().WriteFileAtomic(r.signalsFile, data, signalsFilePermissions)
	}
	if err != nil {
		log.Warn("Failed to write error signals", "path", r.signalsFile, "error", err)
		return
	}
	if r.reporter != nil {
		r.reporter.SignalsWritten(r.signalsFile)
	}
}

// cleanErrorText removes terminal sequences and punctuation noise, then collapses whitespace.
func cleanErrorText(text string) string {
	return strings.Join(strings.Fields(patternNoise.ReplaceAllString(ansi.Strip(text), " ")), " ")
}

// matchesPatterns reports whether the text matches a positive pattern and no negative one. A
// pattern that starts with "!" is negative.
func matchesPatterns(text string, patterns []string) bool {
	matched := false
	for _, pattern := range patterns {
		negative := strings.HasPrefix(pattern, "!")
		expression, err := regexp.Compile(strings.TrimPrefix(pattern, "!"))
		if err != nil || !expression.MatchString(text) {
			continue
		}
		if negative {
			return false
		}
		matched = true
	}
	return matched
}

// sleepContext waits for the duration or until the context ends.
func sleepContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
