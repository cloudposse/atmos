package backend

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgcfn "github.com/cloudposse/atmos/pkg/component/aws/cloudformation"
	"github.com/cloudposse/atmos/pkg/ui"
)

// requireComponentAndStack validates the two selectors every backend verb needs.
// Without this check a missing component surfaces from deep inside stack
// processing as a usage error that names `atmos terraform <command>`, which
// points users at the wrong command group.
func requireComponentAndStack(verb, component, stack string) error {
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}
	if component == "" {
		return errUtils.Build(errUtils.ErrComponentRequired).
			WithExplanationf("`atmos aws cloudformation backend %s` acts on one component's `kind: aws/s3` provision target.", verb).
			WithHintf("Run `atmos aws cloudformation backend %s <component> -s <stack>`.", verb).
			Err()
	}
	return nil
}

// dryRunRequest identifies the backend operation a dry run reports on.
type dryRunRequest struct {
	// Verb is the backend subcommand, for example "create".
	Verb string
	// Action is the phrase describing the effect, for example "create".
	Action    string
	Component string
	Stack     string
	Target    string
}

// executeDryRun resolves the component and its `kind: aws/s3` target from static
// configuration alone, with no authentication and no AWS calls, and reports what
// the verb would do. A nonexistent component or stack, or an unresolvable target,
// fails exactly as the real run would.
func executeDryRun(_ context.Context, req *dryRunRequest) error {
	componentConfig, err := configInit.DescribeComponentStatic(req.Component, req.Stack)
	if err != nil {
		return err
	}
	summary, err := pkgcfn.S3BackendDryRunSummary(componentConfig, req.Target, req.Action)
	if err != nil {
		return err
	}
	ui.Info(fmt.Sprintf("Dry run: backend %s %s in %s: %s; no AWS calls made", req.Verb, req.Component, req.Stack, summary))
	return nil
}
