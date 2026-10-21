package policy

import (
	"context"
	"encoding/json"
)

// InputKind identifies a policy input contract, not a component or CI provider.
type InputKind string

// Well-known input kind names. Collectors and command integration are separate.
const (
	Validation       InputKind = "validation"
	File             InputKind = "file"
	Plan             InputKind = "plan"
	TerraformPlan    InputKind = "terraform.plan"
	Operation        InputKind = "operation"
	Command          InputKind = "command"
	ComponentCommand InputKind = "component.command"
	Auth             InputKind = "auth"
	Function         InputKind = "function"
	Source           InputKind = "source"
	GitChange        InputKind = "git.change"
	AtmosAffected    InputKind = "atmos.affected"
	GitHubEvent      InputKind = "github.event"
	GitHubApproval   InputKind = "github.approval"
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

// Request contains one explicitly selected input kind. File inputs may be any JSON value;
// all other inputs must be JSON objects.
// No input kind implicitly triggers collection of another kind (including affected).
type Request struct {
	Context   *RuntimeContext `json:"context,omitempty"`
	Subject   *Subject        `json:"subject,omitempty"`
	Kind      InputKind       `json:"kind"`
	Input     json.RawMessage `json:"input"`
	CI        *CISnapshot     `json:"ci,omitempty"`
	Revisions Revisions       `json:"revisions"`
	File      string          `json:"file,omitempty"`
}

// Rule is an already resolved definition, not a policy manifest or trust assertion.
// A true predicate selects its effects; false means not applicable, never denial.
type Rule struct {
	Checks       map[string]ContextCheck `json:"checks,omitempty"`
	AllowedRoots []string                `json:"allowed_roots,omitempty"`
	Match        *Selector               `json:"match,omitempty"`
	ID           string                  `json:"id"`
	Description  string                  `json:"description,omitempty"`
	On           InputKind               `json:"on"`
	Engine       string                  `json:"engine"`
	Expression   string                  `json:"expression"`
	Parameters   json.RawMessage         `json:"parameters,omitempty"`
	RequiredCI   []CIRequirement         `json:"required_ci,omitempty"`
	Effects      Effects                 `json:"effects"`
	OPA          *OPAConfig              `json:"opa,omitempty"`
	Schema       *SchemaConfig           `json:"schema,omitempty"`
	Files        *FileSelector           `json:"files,omitempty"`
}

// Effects are obligations selected by a predicate. They cannot perform side effects.
type Effects struct {
	Deny      bool                  `json:"deny"`
	Reason    string                `json:"reason,omitempty"`
	Approvals []ApprovalRequirement `json:"approvals,omitempty"`
	Hooks     []HookRequirement     `json:"hooks,omitempty"`
}

// ApprovalRequirement remains outstanding until a trusted CI resolver verifies it.
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
	RuleID      string         `json:"rule_id"`
	Status      DecisionStatus `json:"status"`
	Effects     Effects        `json:"effects"`
	Error       string         `json:"error,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
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
	Checks     map[string]bool `json:"checks,omitempty"`
	Expression string          `json:"expression"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	Request    *Request        `json:"request"`
	OPA        *OPAConfig      `json:"opa,omitempty"`
	Schema     *SchemaConfig   `json:"schema,omitempty"`
}

// Diagnostic retains a validator's explanation and document location.
// Messages and details may contain input values; callers control their disclosure.
type Diagnostic struct {
	Message    string          `json:"message"`
	Path       string          `json:"path,omitempty"`
	SchemaPath string          `json:"schema_path,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
}

// EvaluationResult extends a predicate match with structured diagnostics.
type EvaluationResult struct {
	Matched     bool         `json:"matched"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// DetailedEvaluator is an optional capability, preserving the boolean Evaluator API.
type DetailedEvaluator interface {
	EvaluateDetailed(ctx context.Context, evaluation Evaluation) (EvaluationResult, error)
}

// Evaluator is the language extension point for pure predicates, including CEL
// and restricted Starlark, without exposing the script automation engine.
//
//go:generate go run go.uber.org/mock/mockgen -source=types.go -destination=mock_evaluator_test.go -package=policy
type Evaluator interface {
	Evaluate(ctx context.Context, evaluation Evaluation) (bool, error)
}
