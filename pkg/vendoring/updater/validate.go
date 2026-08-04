package updater

import (
	"fmt"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Invocation captures the mutually exclusive selector flags a vendor update invocation was called
// with (--pull-request, --all, --group, --component).
type Invocation struct {
	PullRequest bool
	All         bool
	Group       string
	Components  []string
}

// PRTemplates holds the pull-request title/body template text (empty means "use the default").
// Shared by ValidatePullRequestTemplates and RenderPRTemplates so both read the exact same two
// config values.
type PRTemplates struct {
	Title string
	Body  string
}

// ValidationConfig is the plain, viper-free view of the vendor update config this package's
// validation needs -- built once at the CLI boundary in cmd/vendor and passed down here, so this
// package (like its pkg/vendoring siblings) never reads viper directly.
type ValidationConfig struct {
	Format          string
	ExecutionMode   string
	BatchingMode    string
	GroupConfigured bool
	Templates       PRTemplates
}

// ValidateInvocation validates invocation against config, additionally validating the pull-request
// templates unless the invocation is a --pull-request --check dry run (checkExplicitlyRequested),
// which never touches git and so never needs its templates to parse.
func ValidateInvocation(invocation Invocation, config *ValidationConfig, checkExplicitlyRequested bool) error {
	defer perf.Track(nil, "updater.ValidateInvocation")()

	if err := ValidateSelectors(invocation, config); err != nil {
		return err
	}
	if err := ValidateConfiguration(config); err != nil {
		return err
	}
	if invocation.PullRequest && checkExplicitlyRequested {
		// A dry run is allowed with --pull-request, but it deliberately does no
		// branch work. The distinction is surfaced in both terminal and summary.
		return nil
	}
	if invocation.PullRequest {
		return ValidatePullRequestTemplates(config.Templates)
	}
	return nil
}

// ValidateSelectors validates invocation's selector flags are mutually consistent and, when a
// group is given, that it is actually configured.
func ValidateSelectors(invocation Invocation, config *ValidationConfig) error {
	defer perf.Track(nil, "updater.ValidateSelectors")()

	if invocation.All && (invocation.Group != "" || len(invocation.Components) > 0) {
		return fmt.Errorf("%w: --all cannot be used with --group or --component", errUtils.ErrComponentUpdaterConfig)
	}
	if invocation.Group != "" && len(invocation.Components) > 0 {
		return fmt.Errorf("%w: --group and --component cannot be used together", errUtils.ErrComponentUpdaterConfig)
	}
	if f := config.Format; f != "table" && f != "json" {
		return fmt.Errorf("%w: --format must be table or json", errUtils.ErrComponentUpdaterConfig)
	}
	if invocation.Group != "" && !config.GroupConfigured {
		return fmt.Errorf("%w: vendor.update.groups.%s is not configured", errUtils.ErrComponentUpdaterConfig, invocation.Group)
	}
	return nil
}

// ValidateConfiguration validates the execution/batching mode configuration.
//
// Batching.mode only ever accepts "scope" today. Per-component linked-worktree batching was
// previously accepted by this validation and then unconditionally rejected at runtime ("not
// available in this release") -- a materially larger feature (concurrent worktree lifecycle,
// per-component PR fan-out, GitHub rate-limit handling) than plain execution.mode=worktree
// isolation. It's tracked as a deferred, documented future feature in docs/prd/vendor-lock.md's
// Known Limitations, not as something a value here can select.
func ValidateConfiguration(config *ValidationConfig) error {
	defer perf.Track(nil, "updater.ValidateConfiguration")()

	mode := config.ExecutionMode
	if mode != "" && mode != "current" && mode != "worktree" {
		return fmt.Errorf("%w: vendor.update.execution.mode must be current or worktree", errUtils.ErrComponentUpdaterConfig)
	}
	batching := config.BatchingMode
	if batching != "" && batching != "scope" {
		return fmt.Errorf("%w: vendor.update.batching.mode must be scope", errUtils.ErrComponentUpdaterConfig)
	}
	return nil
}

// ValidatePullRequestTemplates confirms templates.Title/Body, when set, are valid Go templates.
func ValidatePullRequestTemplates(templates PRTemplates) error {
	defer perf.Track(nil, "updater.ValidatePullRequestTemplates")()

	for _, text := range []string{templates.Title, templates.Body} {
		if text == "" {
			continue
		}
		if _, err := template.New("component-updater").Funcs(TemplateFunctions()).Parse(text); err != nil {
			return fmt.Errorf("%w: invalid pull request template: %w", errUtils.ErrComponentUpdaterConfig, err)
		}
	}
	return nil
}
