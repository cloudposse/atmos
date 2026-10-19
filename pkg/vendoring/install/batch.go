package install

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/pkg/auth/broker"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/vendoring/concurrency"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
	"github.com/cloudposse/atmos/pkg/vendoring/ordered"
)

// BatchReport keeps outcomes in plan order, including jobs canceled before starting.
type BatchReport struct{ Results []Result }

type batchInstaller struct {
	ctx        context.Context
	config     *schema.AtmosConfiguration
	packages   []VendorPackage
	options    InstallOptions
	observer   batch.Observer
	progressMu sync.Mutex
	needed     map[pkgInstaller]bool
	report     *BatchReport
	failures   []error
}

// InstallBatch prepares packages concurrently and materializes in declaration order.
func InstallBatch(ctx context.Context, config *schema.AtmosConfiguration, packages []VendorPackage, opts InstallOptions, observer batch.Observer) (*BatchReport, error) {
	defer perf.Track(config, "install.InstallBatch")()
	n, err := concurrency.Effective(config, opts.MaxConcurrency)
	if err != nil {
		return nil, err
	}
	b := &batchInstaller{ctx: ctx, config: config, packages: packages, options: opts, observer: observer, report: &BatchReport{Results: make([]Result, len(packages))}}
	for i, pkg := range packages {
		b.report.Results[i] = Result{Name: pkg.Name, Outcome: "canceled"}
	}
	b.emit(&batch.Event{Reset: true, Count: len(packages)})
	if err := b.preflight(); err != nil {
		if ctx.Err() != nil {
			return b.finish(err)
		}
		return nil, err
	}
	err = ordered.Run(ctx, ordered.Options[*PreparedPackage]{
		Count: len(packages), Concurrency: n,
		Barrier: func(i int) bool { return packages[i].PkgType() == PkgTypeLocal || !b.needed[packages[i].installer] },
		Prepare: b.prepare, Commit: b.commit, Dispose: func(p *PreparedPackage) { p.Close() },
	})
	return b.finish(err)
}

func (b *batchInstaller) preflight() error {
	var pending []VendorPackage
	check := func() error {
		var err error
		pending, err = filterPending(b.config, b.packages, b.options, func(message string) { b.emit(&batch.Event{Warning: message}) })
		return err
	}
	// Read receipts and target files consistently with other vendoring writers. Dry-run
	// and refresh do not inspect target state and must not create read-side lock files.
	var err error
	if b.options.DryRun || b.options.RefreshLock || len(b.packages) == 0 {
		err = check()
	} else {
		b.phase(0, "Waiting for vendoring lock")
		err = lockfile.WithMutation(b.ctx, b.config, func() error { b.phase(0, "Checking"); return check() })
	}
	if err != nil {
		return err
	}
	b.needed = make(map[pkgInstaller]bool, len(pending))
	for _, pkg := range pending {
		b.needed[pkg.installer] = true
	}
	if !b.options.DryRun {
		for _, pkg := range b.packages {
			if pkg.PkgType() != PkgTypeLocal {
				broker.EnsureCredentials(b.ctx, b.config)
				break
			}
		}
	}
	return nil
}

func (b *batchInstaller) emit(e *batch.Event) {
	b.progressMu.Lock()
	defer b.progressMu.Unlock()
	if b.observer != nil {
		b.observer(*e)
	}
}

func (b *batchInstaller) phase(i int, phase string) {
	e := batch.Event{ID: i, Label: packageLabel(b.packages[i]), Version: b.packages[i].Version, Phase: phase}
	if phase == "Ready" {
		e.Fraction = preparationWeight
	}
	b.emit(&e)
}

// Preparation and materialization each contribute half of a package's progress.
// This measures milestones, not elapsed time; only a terminal result counts as complete.
const preparationWeight = 0.5

func (b *batchInstaller) downloadProgress(i int, done, total int64) {
	e := batch.Event{ID: i, Bytes: true, Downloaded: done, Total: total}
	if total > 0 {
		e.Fraction = preparationWeight * min(1, max(0, float64(done)/float64(total)))
	}
	b.emit(&e)
}

func (b *batchInstaller) prepare(ctx context.Context, i int) (*PreparedPackage, error) {
	pkg := b.packages[i]
	if !b.needed[pkg.installer] {
		var check lockfile.MaterializationCheck
		b.phase(i, "Waiting for vendoring lock")
		err := lockfile.WithMutation(ctx, b.config, func() error { var err error; check, err = pkg.installer.isMaterialized(b.config); return err })
		if err != nil || check.Materialized {
			return nil, err
		}
	}
	if b.options.DryRun {
		b.phase(i, "Checking")
		return nil, pkg.installer.dryRunCheck(ctx, b.config)
	}
	phase := "Downloading"
	if pkg.PkgType() == PkgTypeLocal {
		phase = "Staging"
	}
	b.phase(i, phase)
	prepared, err := prepareWithProgress(ctx, b.config, pkg, preparationProgress{
		bytes: func(done, total int64) { b.downloadProgress(i, done, total) },
		phase: func(phase string) { b.phase(i, phase) },
		retry: func(attempt int) {
			b.emit(&batch.Event{ID: i, Label: packageLabel(pkg), Version: pkg.Version, Phase: "Retrying", Attempt: attempt})
		},
	})
	if err == nil {
		b.phase(i, "Ready")
	}
	return prepared, err
}

func (b *batchInstaller) commit(i int, prepared *PreparedPackage, err error) error {
	pkg := b.packages[i]
	outcome := "installed"
	if err == nil && prepared != nil {
		b.phase(i, "Waiting to install")
		copyFiles := prepared.copyFiles
		prepared.copyFiles = func() error { b.phase(i, "Installing"); return copyFiles() }
		err = prepared.Materialize(b.ctx, b.config)
	} else if err == nil && !b.options.DryRun {
		outcome = "unchanged"
	}
	if b.options.DryRun {
		outcome = "checked"
	}
	if err != nil {
		outcome = "failed"
		if b.ctx.Err() != nil {
			outcome = "canceled"
		}
		err = fmt.Errorf("%s: %w", pkg.Name, err)
		b.failures = append(b.failures, err)
	}
	b.report.Results[i] = Result{Name: pkg.Name, Err: err, Outcome: outcome}
	b.emit(&batch.Event{ID: i, Label: packageLabel(pkg), Version: pkg.Version, Done: true, Outcome: outcome, Err: err})
	return nil
}

func (b *batchInstaller) finish(err error) (*BatchReport, error) {
	if err != nil {
		b.failures = append(b.failures, err)
		for i, result := range b.report.Results {
			if result.Outcome == "canceled" && result.Err == nil {
				b.emit(&batch.Event{ID: i, Label: packageLabel(b.packages[i]), Version: b.packages[i].Version, Done: true, Outcome: "canceled"})
			}
		}
	}
	return b.report, errors.Join(b.failures...)
}

func packageLabel(pkg VendorPackage) string {
	if pkg.IsMixin() && pkg.MixinFilename() != "" {
		return "mixin " + pkg.MixinFilename()
	}
	return pkg.Name
}
