package generic

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// CreateCheckRun writes check run status to stderr and returns a synthetic CheckRun.
func (p *Provider) CreateCheckRun(_ context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "generic.Provider.CreateCheckRun")()

	ui.Infof("Check run created: %s [%s]", opts.Name, opts.Status)
	if opts.Title != "" {
		ui.Infof("  Title: %s", opts.Title)
	}
	if opts.Summary != "" {
		ui.Infof("  Summary: %s", opts.Summary)
	}

	id := p.nextCheckRunID.Add(1)

	return &provider.CheckRun{
		ID:        id,
		Name:      opts.Name,
		Status:    opts.Status,
		Title:     opts.Title,
		Summary:   opts.Summary,
		StartedAt: time.Now(),
	}, nil
}

// UpdateCheckRun writes check run status to stderr and returns an updated CheckRun.
func (p *Provider) UpdateCheckRun(_ context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	defer perf.Track(nil, "generic.Provider.UpdateCheckRun")()
	var uiMethod func(format string, a ...interface{})
	var verb string
	switch opts.Status {
	case provider.CheckRunStateSuccess:
		uiMethod = ui.Successf
		verb = "completed"
	case provider.CheckRunStateFailure, provider.CheckRunStateError:
		uiMethod = ui.Errorf
		verb = "failed"
	case provider.CheckRunStateCancelled:
		uiMethod = ui.Warningf
		verb = "cancelled"
	default:
		uiMethod = ui.Infof
		verb = "updated"
	}

	uiMethod("Check run %s: %s [%s]", verb, opts.Name, opts.Status)

	if opts.Title != "" {
		uiMethod("  Title: %s", opts.Title)
	}
	if opts.Summary != "" {
		uiMethod("  Summary: %s", opts.Summary)
	}

	return &provider.CheckRun{
		ID:         opts.CheckRunID,
		Name:       opts.Name,
		Status:     opts.Status,
		Conclusion: opts.Conclusion,
		Title:      opts.Title,
		Summary:    opts.Summary,
	}, nil
}
