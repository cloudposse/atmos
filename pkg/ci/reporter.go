package ci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate mockgen -typed -destination=mock_reporter_test.go -package=ci github.com/cloudposse/atmos/pkg/ci Reporter

// Feature names the configuration key that gates a reporting surface.
type Feature string

const (
	// FeatureSummary gates the job step summary.
	FeatureSummary Feature = "ci.summary.enabled"
	// FeatureOutput gates CI outputs, and also env and path exports.
	FeatureOutput Feature = "ci.output.enabled"
	// FeatureAnnotations gates line-anchored annotations.
	FeatureAnnotations Feature = "ci.annotations.enabled"
	// FeatureResults gates SARIF uploads.
	FeatureResults Feature = "ci.results.enabled"
	// FeatureChecks gates commit statuses and check runs.
	FeatureChecks Feature = "ci.checks.enabled"
	// FeatureComments gates PR/MR comments.
	FeatureComments Feature = "ci.comments.enabled"
	// FeatureGroups gates collapsible log groups.
	FeatureGroups Feature = "ci.groups.mode"
	// FeatureEnabled is the CI master switch.
	FeatureEnabled Feature = "ci.enabled"
	// FeatureForkGate is the fork-execution safety gate for elevated events.
	FeatureForkGate Feature = "ci.allow_unsafe_fork_execution"
)

// Receipt describes where a report landed.
type Receipt struct {
	// Provider is the name of the provider that handled the call.
	Provider string
	// Local is true when the local fallback (generic) rendered the report rather than a detected provider.
	Local bool
	// Gate is set when a detected provider was skipped because this switch is off.
	Gate Feature
	// Comment is set by Comment.
	Comment *Comment
	// Check is set by Check and UpdateCheck.
	Check *CheckRun
}

// CommentRequest describes a PR/MR comment to post.
type CommentRequest struct {
	// Body is the comment markdown.
	Body string
	// Key is the marker key. Empty means no marker, and the behavior is forced to create.
	Key string
	// Behavior is create, update, or upsert. Empty means upsert.
	Behavior CommentBehavior
	// PR is the pull request number. Zero means the current PR from Context.
	PR int
}

// CheckRequest describes a commit status or check run to create or update.
type CheckRequest struct {
	// Name is the check run name.
	Name string
	// State is the check run state. Empty means pending.
	State CheckRunState
	// Description is the provider title or status description.
	Description string
	// URL is the details URL. Empty means the run URL from Context.
	URL string
}

// Reporter is the surface scripts and step handlers use to report into CI. Every write
// honors the same configuration switches as native reporting and always has a local
// rendering, so callers never need to know whether a CI provider was detected.
type Reporter interface {
	// Context returns CI run metadata from the detected provider, or the local fallback.
	Context() (*Context, error)
	// Base resolves the base commit for affected detection.
	Base() (*BaseResolution, error)
	// Summary appends markdown to the job summary.
	Summary(markdown string) (Receipt, error)
	// Output writes a CI output variable.
	Output(key, value string) (Receipt, error)
	// Env exports an environment variable to later steps of the same job.
	Env(key, value string) (Receipt, error)
	// Path prepends a directory to PATH for later steps of the same job.
	Path(dir string) (Receipt, error)
	// Mask redacts a value in Atmos output and in the provider's logs.
	Mask(value string) (Receipt, error)
	// Annotate renders a line-anchored annotation.
	Annotate(annotation Annotation) (Receipt, error)
	// Comment posts or updates a PR/MR comment.
	Comment(ctx context.Context, req CommentRequest) (Receipt, error)
	// Check creates a commit status or check run.
	Check(ctx context.Context, req CheckRequest) (Receipt, error)
	// UpdateCheck updates a commit status or check run.
	UpdateCheck(ctx context.Context, req CheckRequest) (Receipt, error)
	// Group opens a collapsible log group. The returned function closes it.
	Group(title string) (end func(), receipt Receipt, err error)
	// SARIF uploads a SARIF report to the provider's findings store.
	SARIF(ctx context.Context, report SARIFReport) (Receipt, error)
	// WithOutput returns a Reporter whose local renderings go to w.
	WithOutput(w io.Writer) Reporter
}

// reporterState is shared between a Reporter and the copies WithOutput returns.
type reporterState struct {
	ctxOnce sync.Once
	ctx     *Context
	ctxErr  error
}

type reporter struct {
	cfg      *schema.AtmosConfiguration
	detected provider.Provider
	fallback provider.Provider
	out      io.Writer
	state    *reporterState
}

// NewReporter returns a Reporter bound to the provider detected in the current
// environment, with the generic provider as the local fallback.
func NewReporter(cfg *schema.AtmosConfiguration) Reporter {
	defer perf.Track(cfg, "ci.NewReporter")()

	fallback, err := Get("generic")
	if err != nil {
		log.Debug("CI reporter has no local fallback provider", "error", err)
		fallback = nil
	}
	return &reporter{
		cfg:      cfg,
		detected: Detect(),
		fallback: fallback,
		state:    &reporterState{},
	}
}

// local returns the fallback provider bound to the reporter's output writer when possible.
func (r *reporter) local() provider.Provider {
	if r.fallback == nil {
		return nil
	}
	if r.out != nil {
		if binder, ok := r.fallback.(provider.OutputBinder); ok {
			return binder.BindOutput(r.out)
		}
	}
	return r.fallback
}

// localReceipt builds a receipt for the local fallback.
func (r *reporter) localReceipt(l provider.Provider, gate Feature) Receipt {
	rc := Receipt{Local: true, Gate: gate}
	if l != nil {
		rc.Provider = l.Name()
	}
	return rc
}

// target picks the provider that handles a write gated by f. It returns a nil provider
// when nothing can render the write.
func (r *reporter) target(f Feature, enabled func(*schema.AtmosConfiguration) bool) (provider.Provider, Receipt) {
	if r.detected == nil {
		l := r.local()
		return l, r.localReceipt(l, "")
	}
	if !enabled(r.cfg) {
		l := r.local()
		return l, r.localReceipt(l, f)
	}
	return r.detected, Receipt{Provider: r.detected.Name()}
}

// routeTo resolves the provider for a write gated by f that needs the optional capability T,
// falling back to the local provider when the chosen one lacks it.
func routeTo[T any](r *reporter, f Feature, enabled func(*schema.AtmosConfiguration) bool, op string) (T, Receipt, bool) {
	var zero T
	p, rc := r.target(f, enabled)
	if p == nil {
		return zero, rc, false
	}
	if c, ok := p.(T); ok {
		return c, rc, true
	}
	if !rc.Local {
		if l := r.local(); l != nil {
			if c, ok := l.(T); ok {
				log.Debug("CI provider lacks capability, using local rendering", "operation", op, "provider", p.Name())
				return c, r.localReceipt(l, rc.Gate), true
			}
		}
	}
	log.Debug("No CI provider supports operation", "operation", op, "provider", p.Name())
	return zero, rc, false
}

// wrapErr wraps err with sentinel unless it already carries it.
func wrapErr(sentinel, err error) error {
	if err == nil || errors.Is(err, sentinel) {
		return err
	}
	return fmt.Errorf("%w: %w", sentinel, err)
}

func (r *reporter) Context() (*Context, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Context")()

	r.state.ctxOnce.Do(func() {
		p := r.detected
		if p == nil {
			p = r.fallback
		}
		if p == nil {
			r.state.ctxErr = errUtils.ErrCIProviderNotDetected
			return
		}
		r.state.ctx, r.state.ctxErr = p.Context()
	})
	return r.state.ctx, r.state.ctxErr
}

func (r *reporter) Base() (*BaseResolution, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Base")()

	p := r.detected
	if p == nil {
		p = r.fallback
	}
	if p == nil {
		return nil, errUtils.ErrCIProviderNotDetected
	}
	// Trust the GitHub Actions workspace before the provider runs git. Mirrors the describe
	// affected path and is gated by ci.enabled because it mutates the runner's git config.
	if r.detected != nil && Enabled(r.cfg) {
		if err := atmosgit.EnsureGitSafeDirectory(); err != nil {
			log.Warn("Failed to configure git safe.directory for GitHub Actions workspace", "error", err)
		}
	}
	return p.ResolveBase()
}

func (r *reporter) Summary(markdown string) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Summary")()

	return r.write(FeatureSummary, SummaryEnabled, errUtils.ErrCISummaryWriteFailed, func(w provider.OutputWriter) error {
		return w.WriteSummary(markdown)
	})
}

func (r *reporter) Output(key, value string) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Output")()

	return r.write(FeatureOutput, OutputEnabled, errUtils.ErrCIOutputWriteFailed, func(w provider.OutputWriter) error {
		return w.WriteOutput(key, value)
	})
}

// write sends a write through the provider's OutputWriter.
func (r *reporter) write(f Feature, enabled func(*schema.AtmosConfiguration) bool, sentinel error, fn func(provider.OutputWriter) error) (Receipt, error) {
	p, rc := r.target(f, enabled)
	if p == nil {
		return rc, nil
	}
	w := p.OutputWriter()
	if w == nil {
		return rc, nil
	}
	return rc, wrapErr(sentinel, fn(w))
}

func (r *reporter) Env(key, value string) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Env")()

	e, rc, ok := routeTo[provider.EnvExporter](r, FeatureOutput, OutputEnabled, "env")
	if !ok {
		return rc, nil
	}
	return rc, wrapErr(errUtils.ErrCIEnvWriteFailed, e.WriteEnv(key, value))
}

func (r *reporter) Path(dir string) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Path")()

	e, rc, ok := routeTo[provider.EnvExporter](r, FeatureOutput, OutputEnabled, "path")
	if !ok {
		return rc, nil
	}
	return rc, wrapErr(errUtils.ErrCIEnvWriteFailed, e.AddPath(dir))
}

func (r *reporter) Mask(value string) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Mask")()

	rc := r.localReceipt(r.local(), "")
	if value == "" {
		return rc, nil
	}
	atmosio.RegisterSecret(value)
	if r.detected == nil || !Enabled(r.cfg) {
		return rc, nil
	}
	m, ok := r.detected.(provider.ValueMasker)
	if !ok {
		return rc, nil
	}
	rc = Receipt{Provider: r.detected.Name()}
	return rc, wrapErr(errUtils.ErrCIMaskFailed, m.MaskValue(value))
}

//nolint:gocritic // The Reporter interface takes Annotation by value to keep the call surface simple.
func (r *reporter) Annotate(annotation Annotation) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Annotate")()

	a, rc, ok := routeTo[provider.Annotator](r, FeatureAnnotations, AnnotationsEnabled, "annotate")
	if !ok {
		return rc, nil
	}
	return rc, wrapErr(errUtils.ErrCIAnnotationFailed, a.Annotate([]Annotation{annotation}))
}

func (r *reporter) SARIF(ctx context.Context, report SARIFReport) (Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.SARIF")()

	s, rc, ok := routeTo[provider.SARIFReporter](r, FeatureResults, ResultsEnabled, "sarif")
	if !ok {
		return rc, nil
	}
	return rc, wrapErr(errUtils.ErrCISARIFUploadFailed, s.ReportSARIF(ctx, report))
}

func (r *reporter) Group(title string) (func(), Receipt, error) {
	defer perf.Track(r.cfg, "ci.Reporter.Group")()

	enabled := func(c *schema.AtmosConfiguration) bool { return resolveGroupMode(c) != GroupModeOff }
	g, rc, ok := routeTo[provider.LogGrouper](r, FeatureGroups, enabled, "group")
	if !ok {
		return func() {}, rc, nil
	}
	if err := g.StartLogGroup(title); err != nil {
		return func() {}, rc, err
	}
	end := func() {
		if err := g.EndLogGroup(); err != nil {
			log.Debug("Failed to close CI log group", "title", title, "error", err)
		}
	}
	return end, rc, nil
}

func (r *reporter) WithOutput(w io.Writer) Reporter {
	defer perf.Track(r.cfg, "ci.Reporter.WithOutput")()

	c := *r
	c.out = w
	return &c
}
