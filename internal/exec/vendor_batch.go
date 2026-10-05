package exec

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

// ExecuteVendorPackages renders a single batch across all selected manifests and types.
func ExecuteVendorPackages(ctx context.Context, config *schema.AtmosConfiguration, packages []install.VendorPackage, opts install.InstallOptions) error {
	defer perf.Track(config, "exec.ExecuteVendorPackages")()
	if len(packages) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	ui.Info("Pulling vendored components")
	var report *install.BatchReport
	err := batch.Run(len(packages), func(emit batch.Observer) error {
		var err error
		report, err = install.InstallBatch(ctx, config, packages, opts, emit)
		return err
	})
	if report != nil {
		counts := map[string]int{}
		for _, r := range report.Results {
			counts[r.Outcome]++
		}
		if opts.DryRun {
			ui.Writef("Checked %d packages, %d failed, %d canceled\n", counts["checked"], counts["failed"], counts["canceled"])
		} else {
			ui.Writef("Vendored %d packages, %d unchanged, %d failed, %d canceled\n", counts["installed"], counts["unchanged"], counts["failed"], counts["canceled"])
		}
	}
	if err != nil {
		return errors.Join(ErrVendorComponents, err)
	}
	return nil
}
