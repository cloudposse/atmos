package policy

import (
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/invopop/jsonschema"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandInput is the canonical command path without argument values.
type CommandInput struct {
	Command string   `json:"command"`
	Path    []string `json:"path"`
}

// ComponentCommandInput describes shared component admission.
type ComponentCommandInput struct {
	Command       string   `json:"command"`
	ComponentType string   `json:"component_type"`
	Component     string   `json:"component"`
	Stack         string   `json:"stack"`
	DryRun        bool     `json:"dry_run"`
	Capabilities  []string `json:"capabilities"`
}

// OperationInput describes a concrete lifecycle event.
type OperationInput struct {
	Event         string `json:"event"`
	ComponentType string `json:"component_type"`
	Command       string `json:"command"`
	Component     string `json:"component"`
	Stack         string `json:"stack"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exit_code"`
	DryRun        bool   `json:"dry_run"`
}

// InputSchema returns the schema for an implemented input, never a proposed capability.
func InputSchema(kind InputKind) (*jsonschema.Schema, error) {
	defer perf.Track(nil, "policy.InputSchema")()

	var value any
	switch kind {
	case Command:
		value = CommandInput{}
	case ComponentCommand:
		value = ComponentCommandInput{}
	case Operation:
		value = OperationInput{}
	case Function:
		value = FunctionInput{}
	case Source:
		value = struct {
			SourceInput
			WithinAllowedRoots *bool `json:"within_allowed_roots,omitempty"`
		}{}
	case AtmosAffected:
		value = AffectedInput{}
	case TerraformPlan:
		value = tfjson.Plan{}
	default:
		return nil, fmt.Errorf("%w: unsupported input kind %q", errUtils.ErrPolicyInvalid, kind)
	}
	r := jsonschema.Reflector{AllowAdditionalProperties: true}
	result := r.Reflect(value)
	result.Title = string(kind) + " policy input"
	return result, nil
}
