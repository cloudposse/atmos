package policy

import (
	"context"
	"encoding/json"
)

// InputKind identifies a policy input contract, not a component or CI provider.
type InputKind string

// Well-known input kind names. Collectors and command integration are separate.
const (
	Validation     InputKind = "validation"
	Plan           InputKind = "plan"
	TerraformPlan  InputKind = "terraform.plan"
	Operation      InputKind = "operation"
	Function       InputKind = "function"
	Source         InputKind = "source"
	GitChange      InputKind = "git.change"
	AtmosAffected  InputKind = "atmos.affected"
	GitHubEvent    InputKind = "github.event"
	GitHubApproval InputKind = "github.approval"
)

// DecisionStatus distinguishes authorization from missing evidence and errors.
type DecisionStatus string

const (
	Allow            DecisionStatus = "allow"
	Deny             DecisionStatus = "deny"
	AwaitingApproval DecisionStatus = "awaiting_approval"
	NotApplicable    DecisionStatus = "not_applicable"
	EvaluationError  DecisionStatus = "error"
)

// Revisions keeps the governing policy revision separate from the comparison base.
// Values are supplied by a trusted caller; the engine does not verify Git objects.
type Revisions struct {
	Governing string `json:"governing"`
	Base      string `json:"base"`
	Head      string `json:"head"`
}

// Request contains one explicitly selected input kind. Input must be a JSON object.
// No input kind implicitly triggers collection of another kind (including affected).
type Request struct {
	Kind      InputKind       `json:"kind"`
	Input     json.RawMessage `json:"input"`
	CI        *CISnapshot     `json:"ci,omitempty"`
	Revisions Revisions       `json:"revisions"`
}

// Rule is an already resolved definition, not a policy manifest or trust assertion.
// A true predicate selects its effects; false means not applicable, never denial.
type Rule struct {
	ID          string          `json:"id"`
	Description string          `json:"description,omitempty"`
	On          InputKind       `json:"on"`
	Engine      string          `json:"engine"`
	Expression  string          `json:"expression"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	RequiredCI  []CIRequirement `json:"required_ci,omitempty"`
	Effects     Effects         `json:"effects"`
}

// Effects are obligations selected by a predicate. They cannot perform side effects.
type Effects struct {
	Deny      bool                  `json:"deny"`
	Reason    string                `json:"reason,omitempty"`
	Approvals []ApprovalRequirement `json:"approvals,omitempty"`
	Hooks     []HookRequirement     `json:"hooks,omitempty"`
}

// ApprovalRequirement remains outstanding until a future trusted resolver verifies it.
// Spec is provider-owned data, not a universal team/review vocabulary.
type ApprovalRequirement struct {
	ID       string          `json:"id"`
	Provider string          `json:"provider"`
	Spec     json.RawMessage `json:"spec"`
}

// HookRequirement references a trusted hook definition resolved by the caller.
// Required (must execute) and RequireSuccess (must succeed if executed) are distinct.
// Neither an allow decision nor a selected hook is evidence of hook completion.
type HookRequirement struct {
	Ref            string `json:"ref" yaml:"ref"`
	Event          string `json:"event" yaml:"event"`
	Required       bool   `json:"required" yaml:"required"`
	RequireSuccess bool   `json:"require_success" yaml:"require_success"`
}

// Finding preserves each rule's outcome and obligations, including on aggregate denial.
type Finding struct {
	RuleID  string         `json:"rule_id"`
	Status  DecisionStatus `json:"status"`
	Effects Effects        `json:"effects"`
	Error   string         `json:"error,omitempty"`
}

// Decision is a pure evaluation result, not a final CI or operation result.
// Error dominates deny, then awaiting approval, allow, and not applicable.
type Decision struct {
	Status   DecisionStatus `json:"status"`
	Findings []Finding      `json:"findings"`
}

// Evaluation contains detached, materialized data for a single predicate.
// Adapters must be concurrency-safe, honor cancellation, and perform no I/O.
type Evaluation struct {
	Expression string          `json:"expression"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	Request    *Request        `json:"request"`
}

// Evaluator is the language extension point. Future restricted Starlark support
// can implement this contract without exposing the script automation engine.
//
//go:generate go run go.uber.org/mock/mockgen -source=types.go -destination=mock_evaluator_test.go -package=policy
type Evaluator interface {
	Evaluate(ctx context.Context, evaluation Evaluation) (bool, error)
}
