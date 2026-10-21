package policy

import (
	"fmt"
	"strconv"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/invopop/jsonschema"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandInput is the canonical command path without argument values.
// Destroy is the only value derived from arguments: a normalized boolean, see IsDestroyInvocation.
type CommandInput struct {
	Command string   `json:"command"`
	Path    []string `json:"path"`
	Destroy bool     `json:"destroy"`
}

// ComponentCommandInput describes shared component admission.
type ComponentCommandInput struct {
	Command       string   `json:"command"`
	ComponentType string   `json:"component_type"`
	Component     string   `json:"component"`
	Stack         string   `json:"stack"`
	DryRun        bool     `json:"dry_run"`
	Destroy       bool     `json:"destroy"`
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
	case File:
		return &jsonschema.Schema{Title: "file policy input"}, nil
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

const (
	destroyName         = "destroy"
	terraformFamilyName = "terraform"
	helmfileFamilyName  = "helmfile"
)

// IsDestroyInvocation reports whether a command invocation destroys infrastructure.
// It derives the boolean exposed as `destroy` in command and component.command
// policy inputs, so rule authors never parse argument values themselves.
//
// The path is the canonical command path (for example ["terraform", "apply"]), or the
// component type followed by the subcommand. The args are the raw command-line
// arguments of the invocation (positionals and flags in any order). They are only
// inspected for the Terraform family, and only for the `destroy` flag; no other
// argument value is derived or exposed.
//
// The result is true when:
//   - the path is `terraform destroy` or `helmfile destroy`, or
//   - the path belongs to the terraform family (OpenTofu shares it) and any argument
//     is the destroy flag: -destroy or --destroy, with or without a boolean value
//     (-destroy=true, --destroy=1, -destroy=T, ...), including after the `--` separator.
//
// A false boolean value such as -destroy=false, -destroy=0, or -destroy=F does not make a
// command a destroy. Values are parsed with strconv.ParseBool, like Go's flag package.
// The check fails closed: an unparseable value counts as true, and any true occurrence
// wins over a later false one, because the final argument order seen by the tool can
// differ from the order typed.
//
// It cannot see destruction that was decided earlier, such as applying a saved plan
// created with `plan -destroy`. Terraform plan policies cover that case.
func IsDestroyInvocation(path, args []string) bool {
	defer perf.Track(nil, "policy.IsDestroyInvocation")()

	if len(path) == 0 {
		return false
	}
	switch path[0] {
	case helmfileFamilyName:
		return len(path) > 1 && path[1] == destroyName
	case terraformFamilyName:
		if len(path) > 1 && path[1] == destroyName {
			return true
		}
		return hasDestroyFlag(args)
	default:
		return false
	}
}

// hasDestroyFlag reports whether any argument is a truthy destroy flag.
func hasDestroyFlag(args []string) bool {
	for _, arg := range args {
		if isTruthyDestroyFlag(arg) {
			return true
		}
	}
	return false
}

// isTruthyDestroyFlag parses one argument as -destroy[=value] or --destroy[=value].
func isTruthyDestroyFlag(arg string) bool {
	name, ok := strings.CutPrefix(arg, "--")
	if !ok {
		name, ok = strings.CutPrefix(arg, "-")
	}
	if !ok {
		return false
	}
	name, value, hasValue := strings.Cut(name, "=")
	if name != destroyName {
		return false
	}
	if !hasValue {
		return true
	}
	enabled, err := strconv.ParseBool(value)
	return err != nil || enabled
}
