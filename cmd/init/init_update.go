package initcmd

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	gen "github.com/cloudposse/atmos/pkg/generator"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/storage"
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

// This file holds executeInit's post-selection execution and
// --update-retry-offer orchestration (split out of init.go to keep it under
// the repo's file-length-limit -- see CLAUDE.md's File Organization
// section): running the selected template against the resolved target
// directory, and -- when that first attempt fails because the target
// already exists and is non-empty -- offering to retry it as a 3-way-merge
// update instead of just failing outright.

// runInitExecution executes the init with the selected template and target directory.
func runInitExecution(initUI InitUI, selectedConfig *templates.Configuration, opts *initOptions) (string, error) {
	// If target directory is empty, use interactive flow; otherwise use normal Execute.
	if opts.targetDir == "" {
		return runInitInteractiveFlow(initUI, selectedConfig, opts)
	}
	return runInitTargetedFlow(initUI, selectedConfig, opts)
}

// runInitInteractiveFlow handles init when no target directory was provided,
// prompting the user for one (and optionally offering a 3-way-merge update
// instead of failing when it already exists and is non-empty).
func runInitInteractiveFlow(initUI InitUI, selectedConfig *templates.Configuration, opts *initOptions) (string, error) {
	if !opts.interactive {
		return "", fmt.Errorf("%w: target directory is required in non-interactive mode", errUtils.ErrInitialization)
	}

	resolved, err := resolveInteractiveInitBaseRef(initUI, selectedConfig, opts)
	if resolved.cleanup != nil {
		defer resolved.cleanup()
	}
	if err != nil {
		return resolved.targetDir, err
	}

	finalTargetDir, err := initUI.ExecuteWithInteractiveFlowAndBaseRefResult(
		selectedConfig, resolved.targetDir, opts.force, opts.update, resolved.useDefaults, resolved.baseRef, resolved.templateValues,
	)
	offer, retryBaseRef, offerErr := shouldOfferUpdate(err, opts, finalTargetDir)
	if offerErr != nil {
		return finalTargetDir, offerErr
	}
	if offer {
		if confirmed, cErr := initUI.ConfirmUpdateInstead(finalTargetDir); cErr == nil && confirmed {
			renderedCleanup, prepErr := prepareRenderedRetryBase(initUI, opts, finalTargetDir)
			if renderedCleanup != nil {
				defer renderedCleanup()
			}
			if prepErr != nil {
				return finalTargetDir, prepErr
			}
			return initUI.ExecuteWithInteractiveFlowAndBaseRefResult(
				selectedConfig, finalTargetDir, opts.force, true, resolved.useDefaults, retryBaseRef, resolved.templateValues,
			)
		}
	}
	return finalTargetDir, err
}

// prepareRenderedRetryBase resolves and wires the rendered update-strategy's
// base config before a "confirm update instead" retry, mirroring
// cmd/scaffold's helper of the same name. Note that executeInit's and
// resolveInteractiveInitBaseRef's normal opts.update-gated
// ResolveRenderedBase/SetRenderedBaseSource setup only runs when --update
// was passed up front; the retry flips update=true only after the initial
// (non-update) attempt already failed with ErrTargetDirectoryNotEmpty, so
// that setup never ran for this call. Without it, the retry's
// ExecuteWithBaseRef/ExecuteWithInteractiveFlowAndBaseRefResult call would
// reach setupUpdateBase's rendered branch with no base source ever
// configured. Returns a nil cleanup when the strategy isn't rendered or
// resolution failed -- callers must nil-check before deferring it.
func prepareRenderedRetryBase(initUI InitUI, opts *initOptions, targetDir string) (cleanup func(), err error) {
	updateStrategy, err := engine.ParseUpdateStrategy(opts.updateStrategy)
	if err != nil {
		return nil, err
	}
	if updateStrategy != engine.UpdateStrategyRendered {
		// Tracked-strategy retries skip resolveInteractiveInitBaseRef's/
		// executeInit's normal opts.update-gated strategy-switch check for
		// the same reason they skip the rendered base setup above: that
		// check only runs when --update was passed up front, and this retry
		// flips update=true only after the fact. Run it here so a target
		// last managed with --update-strategy=rendered still gets flagged
		// instead of silently retried against stale or absent git history.
		if err := source.CheckNotSwitchedFromRendered(targetDir); err != nil {
			return nil, err
		}
		return nil, nil
	}
	renderedBase, err := source.ResolveRenderedBase(targetDir, opts.sourceOverride)
	if err != nil {
		return nil, err
	}
	initUI.SetRenderedBaseSource(renderedBase.Config, renderedBase.Values)
	return renderedBase.Cleanup, nil
}

// interactiveInitBaseRef bundles resolveInteractiveInitBaseRef's results
// (grouped into a struct, rather than five separate return values, to stay
// under revive's function-result-limit).
type interactiveInitBaseRef struct {
	targetDir      string
	baseRef        string
	templateValues map[string]interface{}
	useDefaults    bool
	// cleanup releases the update-strategy=rendered old-ref source fetch
	// (see source.ResolveRenderedBase), if one was made. nil otherwise --
	// callers must nil-check before invoking it.
	cleanup func()
}

// resolveInteractiveInitBaseRef resolves the --update merge base ref for the
// no-positional-target interactive flow, mirroring cmd/scaffold's
// resolveInteractiveBaseRef. --base-ref's default (the pinned ref from
// .atmos/init/metadata.yaml, see defaultBaseRef) can only be looked up once
// the real target directory is known, but in this flow that directory
// doesn't exist until the interactive prompt below picks one -- so for
// --update, resolve the target directory first (initUI.ResolveTargetPath
// runs the same prompt/setup-form logic
// ExecuteWithInteractiveFlowAndBaseRefResult would, and is a no-op once
// targetDir is non-empty), then resolve the base ref against it, and
// finally hand both back to the caller's
// ExecuteWithInteractiveFlowAndBaseRefResult call -- which skips prompting
// again since targetDir is already set.
//
// Without --update the base ref is unused (ExecuteWithDelimiters only sets
// up git storage when update is true), so this is a no-op passthrough that
// still lets the interactive flow prompt for the target itself.
func resolveInteractiveInitBaseRef(
	initUI InitUI,
	selectedConfig *templates.Configuration,
	opts *initOptions,
) (interactiveInitBaseRef, error) {
	if !opts.update {
		return interactiveInitBaseRef{baseRef: opts.baseRef, templateValues: opts.templateVars, useDefaults: !opts.interactive}, nil
	}

	targetDir, templateValues, useDefaults, err := initUI.ResolveTargetPath(selectedConfig, "", opts.update, !opts.interactive, opts.templateVars)
	if err != nil {
		return interactiveInitBaseRef{targetDir: targetDir}, err
	}

	// engine.UpdateStrategyRendered's base ref comes from the target's own
	// recorded scaffold.yaml (see source.ResolveRenderedBase), not --base-ref
	// -- mirrored here for the no-positional-target flow the same way
	// executeInit already handles it for the positional-target flow, since
	// targetDir only becomes known at this point in this flow.
	updateStrategy, err := engine.ParseUpdateStrategy(opts.updateStrategy)
	if err != nil {
		return interactiveInitBaseRef{targetDir: targetDir}, err
	}
	var cleanup func()
	if updateStrategy == engine.UpdateStrategyRendered {
		renderedBase, srcErr := source.ResolveRenderedBase(targetDir, opts.sourceOverride)
		if srcErr != nil {
			return interactiveInitBaseRef{targetDir: targetDir}, srcErr
		}
		cleanup = renderedBase.Cleanup
		initUI.SetRenderedBaseSource(renderedBase.Config, renderedBase.Values)
	}

	// Skipped under rendered for the same reason as executeInit's positional
	// flow: this resolution's result flows through to spec.baseRef, the
	// same project-record field ResolveRenderedBase's spec.renderedRef
	// counterpart uses to detect a tracked/rendered strategy switch.
	if updateStrategy == engine.UpdateStrategyRendered {
		return interactiveInitBaseRef{targetDir: targetDir, templateValues: templateValues, useDefaults: useDefaults, cleanup: cleanup}, nil
	}

	if err := source.CheckNotSwitchedFromRendered(targetDir); err != nil {
		return interactiveInitBaseRef{targetDir: targetDir, cleanup: cleanup}, err
	}
	baseRef, err := defaultBaseRef(opts.baseRef, targetDir)
	if err != nil {
		return interactiveInitBaseRef{targetDir: targetDir, cleanup: cleanup}, err
	}
	return interactiveInitBaseRef{targetDir: targetDir, baseRef: baseRef, templateValues: templateValues, useDefaults: useDefaults, cleanup: cleanup}, nil
}

// runInitTargetedFlow handles init when a target directory was provided
// (offering the same 3-way-merge update fallback as the interactive flow).
func runInitTargetedFlow(initUI InitUI, selectedConfig *templates.Configuration, opts *initOptions) (string, error) {
	err := initUI.ExecuteWithBaseRef(selectedConfig, opts.targetDir, opts.force, opts.update, !opts.interactive, opts.baseRef, opts.templateVars)
	offer, retryBaseRef, offerErr := shouldOfferUpdate(err, opts, opts.targetDir)
	if offerErr != nil {
		return opts.targetDir, offerErr
	}
	if offer {
		if confirmed, cErr := initUI.ConfirmUpdateInstead(opts.targetDir); cErr == nil && confirmed {
			renderedCleanup, prepErr := prepareRenderedRetryBase(initUI, opts, opts.targetDir)
			if renderedCleanup != nil {
				defer renderedCleanup()
			}
			if prepErr != nil {
				return opts.targetDir, prepErr
			}
			return opts.targetDir, initUI.ExecuteWithBaseRef(selectedConfig, opts.targetDir, opts.force, true, !opts.interactive, retryBaseRef, opts.templateVars)
		}
	}
	return opts.targetDir, err
}

// shouldOfferUpdate decides whether to offer a 3-way-merge update instead of
// failing outright on a non-empty target directory: only when the failure is
// exactly that, the caller isn't already using --force/--update, and a real
// terminal is available to prompt on. TargetDir must be the actual, final
// target directory generation just ran against (not opts.targetDir, which is
// the raw positional arg and can be "" when the interactive flow picked the
// real directory itself -- see resolveInteractiveInitBaseRef). Returns the
// base ref to retry with (the caller's --base-ref, defaulting to HEAD or a
// pinned metadata ref) alongside the decision.
//
// Under --update-strategy=rendered the retry base ref is always "": tracked's
// defaultBaseRef resolution (reading .atmos/init/metadata.yaml) is
// tracked-mode-specific bookkeeping that has no meaning for rendered, and its
// non-empty result would otherwise flow unchanged into the retry's
// executeWithSetup call, which sets spec.baseRef from whatever baseRef it's
// given regardless of strategy -- the same project-record pollution
// CheckNotSwitchedFromRendered exists to guard against, just reached through
// this offer-a-retry path instead of an explicit --update.
func shouldOfferUpdate(err error, opts *initOptions, targetDir string) (offer bool, baseRef string, resolveErr error) {
	if err == nil || opts.force || opts.update || !opts.interactive {
		return false, "", nil
	}
	if !errors.Is(err, errUtils.ErrTargetDirectoryNotEmpty) {
		return false, "", nil
	}
	updateStrategy, resolveErr := engine.ParseUpdateStrategy(opts.updateStrategy)
	if resolveErr != nil {
		return false, "", resolveErr
	}
	if updateStrategy == engine.UpdateStrategyRendered {
		return true, "", nil
	}
	resolvedBaseRef, resolveErr := defaultBaseRef(opts.baseRef, targetDir)
	if resolveErr != nil {
		return false, "", resolveErr
	}
	return true, resolvedBaseRef, nil
}

// defaultBaseRef resolves init's --update base ref against this target's own
// pinned metadata (.atmos/init/metadata.yaml, written by
// gen.PinInitialBaseRefForInit). See gen.ResolveDefaultBaseRef's doc for the
// full rationale -- that function is shared with cmd/scaffold's equivalent
// so the two commands' base-ref resolution can't drift apart again (as it
// did before: this command shipped with the same silent-overwrite bug
// cmd/scaffold fixed, because the fix lived only in cmd/scaffold and was
// never ported here).
func defaultBaseRef(baseRef, targetDir string) (string, error) {
	return gen.ResolveDefaultBaseRef(baseRef, targetDir, storage.InitMetadataPath(targetDir))
}
