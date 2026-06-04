package hooks

import "strings"

// HookEvent identifies a lifecycle moment at which hooks may fire.
type HookEvent string

// Lifecycle phases. Events are named `<phase>.<kind>.<subcommand>` — e.g.
// `after.terraform.apply` for the built-in terraform type, or
// `after.agent.greeting` for a custom component type named `agent` invoked via a
// `greeting` subcommand. The phase is the leading segment.
const (
	PhaseBefore = "before"
	PhaseAfter  = "after"
)

const (
	// Built-in terraform lifecycle events. These follow the same
	// `<phase>.<kind>.<subcommand>` shape that custom component types use; the
	// kind here is just `terraform`.
	BeforeTerraformInit   HookEvent = "before.terraform.init"
	BeforeTerraformApply  HookEvent = "before.terraform.apply"
	AfterTerraformApply   HookEvent = "after.terraform.apply"
	BeforeTerraformPlan   HookEvent = "before.terraform.plan"
	AfterTerraformPlan    HookEvent = "after.terraform.plan"
	BeforeTerraformDeploy HookEvent = "before.terraform.deploy"
	AfterTerraformDeploy  HookEvent = "after.terraform.deploy"
)

// ComponentEvent builds a lifecycle event name for a component kind and the
// subcommand operating on it, e.g.
//
//	ComponentEvent(PhaseAfter, "agent", "greeting") => "after.agent.greeting"
//
// This deliberately mirrors the built-in terraform events
// (`after.terraform.apply`, …) so custom component types are first-class
// citizens of the same taxonomy rather than being anchored to terraform's
// `apply` verb.
func ComponentEvent(phase, componentType, subcommand string) HookEvent {
	return HookEvent(strings.Join([]string{phase, componentType, subcommand}, "."))
}

// NormalizeEvent canonicalizes a user-provided event string into a HookEvent.
//
// User configs conventionally use dash-separated names (`after-terraform-apply`,
// `after-agent-greeting`); the engine internally uses dot-separated names
// (`after.terraform.apply`). This converts dashes to dots so the two spellings
// compare equal. Because the same conversion is applied to both the fired event
// and each configured event before comparison, the mapping only needs to be
// consistent — not reversible — so component/subcommand names that themselves
// contain dashes (e.g. `deploy-app`) still match correctly.
func NormalizeEvent(s string) HookEvent {
	return HookEvent(strings.ReplaceAll(strings.TrimSpace(s), "-", "."))
}

// Normalize returns the canonical form of a HookEvent, collapsing deploy aliases
// to their apply equivalents. deploy and apply are semantically equivalent —
// deploy is apply with -auto-approve — so hooks configured for either should
// fire regardless of which command the user runs.
func (e HookEvent) Normalize() HookEvent {
	switch e {
	case AfterTerraformDeploy:
		return AfterTerraformApply
	case BeforeTerraformDeploy:
		return BeforeTerraformApply
	default:
		return e
	}
}

// IsPostExecution reports whether the event fires after terraform has already run
// (and therefore after terraform init has already completed).
// Store hooks use this to decide whether to skip terraform init when reading outputs:
// after-events can safely skip init because the workdir is already initialized;
// before-events must run init because the workdir may not be initialized yet.
func (e HookEvent) IsPostExecution() bool {
	return strings.HasPrefix(string(e), "after.")
}
