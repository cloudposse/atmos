package stack

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// executeLifecyclePlans validates the complete operation before launching any
// children. Deleted definitions cannot be passed to ordinary component commands:
// they require the isolated recovery workflow from the deleted-component PRD.
func executeLifecyclePlans(cmd *cobra.Command, run *lifecycleRun, affected []schema.Affected) error {
	plan, globalArgs, opts := run.plan, run.globalArgs, run.options
	var blocked []string
	if opts.allowDestroy {
		for i := range affected {
			target := &affected[i]
			if target.Deleted && (opts.stack == "" || opts.stack == target.Stack) {
				blocked = append(blocked, target.ComponentType+"/"+target.Stack+"/"+target.Component)
			}
		}
	}
	if len(blocked) > 0 {
		for _, target := range blocked {
			ui.Infof("Blocked destroy %s: deleted-definition recovery is not yet available", target)
		}
		if opts.dryRun {
			if err := previewLifecycle(plan, opts.verb, globalArgs, cmd); err != nil {
				return err
			}
		}
		return fmt.Errorf("%w: --allow-destroy requires deleted-definition recovery for %s; no component operations were started", errUtils.ErrInvalidConfig, strings.Join(blocked, ", "))
	}
	return runLifecyclePlan(cmd, run)
}
