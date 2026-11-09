package exec

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AdmitComponent evaluates component admission (policy `component.command`) for
// a component invocation without templates, YAML functions, or credentials, so a
// denied command fails before Atmos logs in to any identity. It resolves a copy of
// info and returns only policy admission failures: other resolution errors are left
// to the executor, which reports them authoritatively. It does nothing when no
// component admission rules are configured or the component/stack is not known yet.
func AdmitComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, authManager auth.AuthManager) error {
	defer perf.Track(atmosConfig, "exec.AdmitComponent")()

	if info.ComponentFromArg == "" || info.Stack == "" {
		return nil
	}
	enabled, err := admission.HasRules(policy.ComponentCommand)
	if err != nil || !enabled {
		return err
	}
	if _, err := ProcessStacks(atmosConfig, *info, true, false, false, nil, authManager); policy.IsAdmissionError(err) {
		return err
	}
	return nil
}

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
		Command: info.SubCommand, CommandPath: componentCommandPath(info), ComponentType: info.ComponentType, Component: info.ComponentFromArg, Stack: info.Stack, DryRun: info.DryRun,
		Destroy:      policy.IsDestroyInvocation([]string{info.ComponentType, info.SubCommand}, args),
		Capabilities: componentPolicyCapabilities(info.ComponentType),
	})
	if err != nil {
		return err
	}
	return admission.Check(context.Background(), &policy.Request{Kind: policy.ComponentCommand, Subject: subject, Input: data})
}

// componentCommandPath returns the CLI command path of a component invocation. Callers
// that set CliArgs (executors, describe, secret commands) name it explicitly; component
// commands admitted before their executor runs fall back to the component type and
// subcommands, which is the same path their executor records.
func componentCommandPath(info *schema.ConfigAndStacksInfo) []string {
	path := info.CliArgs
	if len(path) == 0 {
		path = []string{info.ComponentType, info.SubCommand, info.SubCommand2}
	}
	return slices.DeleteFunc(slices.Clone(path), func(s string) bool { return s == "" })
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
