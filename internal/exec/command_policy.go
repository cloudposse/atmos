package exec

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// enforceComponentAdmission runs for all component types using stack resolution,
// before templates/functions, generators, hooks, or component tool execution.
func enforceComponentAdmission(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	enabled, err := admission.HasRules(policy.ComponentCommand)
	if err != nil || !enabled {
		return err
	}
	subject, err := policy.NewSubject(policy.ObjectIdentity{
		Name: info.ComponentFromArg, Stack: info.Stack, ComponentType: info.ComponentType,
	}, info.ComponentSection["metadata"], config.Templates.Settings.Delimiters...)
	if err != nil {
		return err
	}
	// Terraform pass-through flags (for example -destroy) are already folded into the info.
	args := slices.Concat(info.CliArgs, info.AdditionalArgsAndFlags)
	data, err := json.Marshal(policy.ComponentCommandInput{
		Agents:  admission.AgentNames(),
		Command: info.SubCommand, ComponentType: info.ComponentType, Component: info.ComponentFromArg, Stack: info.Stack, DryRun: info.DryRun,
		Destroy:      policy.IsDestroyInvocation([]string{info.ComponentType, info.SubCommand}, args),
		Capabilities: componentPolicyCapabilities(info.ComponentType),
	})
	if err != nil {
		return err
	}
	return admission.Check(context.Background(), &policy.Request{Kind: policy.ComponentCommand, Subject: subject, Input: data})
}

func componentPolicyCapabilities(componentType string) []string {
	capabilities := []string{"component.command", "function", "source"}
	switch componentType {
	case "terraform":
		return append(capabilities, "operation", "terraform.plan")
	case "helm", "helmfile", "kubernetes":
		return append(capabilities, "operation")
	default:
		return capabilities
	}
}
