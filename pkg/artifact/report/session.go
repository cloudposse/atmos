package report

import (
	"errors"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// pending is one buffered entry with its arrival number.
type pending struct {
	entry   *Entry
	arrival int
}

// Session is the collector that renders. It buffers the entries recorded during one execution
// (a CLI command, a workflow, a custom command, or a hook), keeps the final attempt of each
// logical operation, and on [Session.Flush] writes one deterministically ordered report through
// the CI reporter. It is safe for concurrent use.
//
// A session whose reporting surfaces are all disabled ignores every entry, so the gates cost
// nothing. It never calls a service or repository.
type Session struct {
	config      *schema.AtmosConfiguration
	newReporter func(*schema.AtmosConfiguration) ci.Reporter
	maxBytes    int

	summaryEnabled bool
	outputEnabled  bool

	mu      sync.Mutex
	entries map[string]*pending
	arrival int
}

// Option configures a [Session].
type Option func(*Session)

// WithReporter replaces the factory that builds the CI reporter. The default is ci.NewReporter,
// which owns provider detection and the ci.* gating. Tests inject a fake reporter.
func WithReporter(factory func(*schema.AtmosConfiguration) ci.Reporter) Option {
	defer perf.Track(nil, "report.WithReporter")()

	return func(s *Session) {
		if factory != nil {
			s.newReporter = factory
		}
	}
}

// WithMaxBytes sets the largest summary the session writes. Zero or a negative value keeps
// [DefaultMaxBytes].
func WithMaxBytes(limit int) Option {
	defer perf.Track(nil, "report.WithMaxBytes")()

	return func(s *Session) {
		if limit > 0 {
			s.maxBytes = limit
		}
	}
}

// NewSession returns a session that reports according to the ci.* gates of config. A nil config
// disables every surface.
func NewSession(config *schema.AtmosConfiguration, options ...Option) *Session {
	defer perf.Track(config, "report.NewSession")()

	session := &Session{
		config:         config,
		newReporter:    ci.NewReporter,
		maxBytes:       DefaultMaxBytes,
		summaryEnabled: ci.SummaryEnabled(config),
		outputEnabled:  ci.OutputEnabled(config),
		entries:        make(map[string]*pending),
	}
	for _, option := range options {
		option(session)
	}
	return session
}

// Enabled reports whether the session reports anything. A disabled session ignores every entry.
func (s *Session) Enabled() bool {
	defer perf.Track(s.config, "report.Session.Enabled")()

	return s.summaryEnabled || s.outputEnabled
}

// Record implements [Collector]. Attempts of one logical operation share an OperationID; the
// attempt with the highest number replaces the earlier ones, and a later arrival wins a tie. An
// entry without an operation ID is its own operation.
func (s *Session) Record(entry *Entry) {
	defer perf.Track(s.config, "report.Session.Record")()

	if entry == nil || !s.Enabled() {
		return
	}
	copied := *entry
	s.mu.Lock()
	defer s.mu.Unlock()

	s.arrival++
	key := entry.OperationID
	if key == "" {
		key = fmt.Sprintf("anonymous-%d", s.arrival)
	}
	existing, found := s.entries[key]
	switch {
	case !found:
		s.entries[key] = &pending{entry: &copied, arrival: s.arrival}
	case copied.Attempt >= existing.entry.Attempt:
		// The first sighting fixes the arrival number, so a retry never moves the operation.
		if copied.Position == nil {
			copied.Position = existing.entry.Position
		}
		existing.entry = &copied
	}
}

// drain returns the buffered entries and empties the buffer, so a second flush never repeats them.
func (s *Session) drain() []*pending {
	s.mu.Lock()
	defer s.mu.Unlock()

	drained := make([]*pending, 0, len(s.entries))
	for _, item := range s.entries {
		drained = append(drained, item)
	}
	s.entries = make(map[string]*pending)
	return drained
}

// Flush writes the report for everything recorded since the last flush: one summary through the CI
// reporter and, when output variables are enabled, one set of output variables. It returns the
// reporting error, if any; the caller logs it and never lets it change the artifact exit status.
// Flush is meant for a deferred call at the end of an execution, so it runs on failure paths too.
func (s *Session) Flush() error {
	defer perf.Track(s.config, "report.Session.Flush")()

	drained := s.drain()
	if len(drained) == 0 {
		return nil
	}
	entries := orderEntries(drained)
	reporter := s.newReporter(s.config)

	var errs []error
	if s.summaryEnabled {
		errs = append(errs, s.writeSummary(reporter, entries))
	}
	if s.outputEnabled {
		errs = append(errs, writeOutputs(reporter, entries))
	}
	return errors.Join(errs...)
}

// FlushAndLog flushes the session and logs a reporting failure at Warn level. Reporting is
// best-effort: the failure never reaches the caller, so it cannot change an exit status.
func (s *Session) FlushAndLog() {
	defer perf.Track(s.config, "report.Session.FlushAndLog")()

	if err := s.Flush(); err != nil {
		log.Warn("Artifact report was not written", "error", err)
	}
}

// writeSummary renders the report and appends it to the job summary.
func (s *Session) writeSummary(reporter ci.Reporter, entries []*Entry) error {
	set, err := newTemplateSet(s.config)
	if err != nil {
		return err
	}
	report, operations := assemble(entries)
	markdown, err := renderAll(set, report, operations, s.maxBytes)
	if err != nil {
		return err
	}
	if _, err := reporter.Summary(markdown); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrArtifactReportWrite, err)
	}
	return nil
}
