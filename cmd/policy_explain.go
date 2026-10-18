package cmd

import (
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/ui"
)

// policyExplainRequested reads the boolean before project config can execute functions.
func policyExplainRequested(args []string) bool {
	enabled := false
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--explain" {
			enabled = true
			// A following true or false literal is the flag's value, as in the rest of the pipeline.
			if value, ok := explainLiteral(args, i+1); ok {
				enabled = value
			}
		}
		if strings.HasPrefix(arg, "--explain=") {
			enabled, _ = strconv.ParseBool(strings.TrimPrefix(arg, "--explain="))
		}
	}
	return enabled
}

// explainLiteral reads the true or false literal at args[index], which follows a bare --explain.
func explainLiteral(args []string, index int) (value, ok bool) {
	if index >= len(args) {
		return false, false
	}
	switch {
	case strings.EqualFold(args[index], "true"):
		return true, true
	case strings.EqualFold(args[index], "false"):
		return false, true
	}
	return false, false
}

func beginPolicyExplanation(args []string) (func(), error) {
	if !policyExplainRequested(args) {
		return func() {}, nil
	}
	bundle, err := admission.LoadBundle()
	if err != nil {
		return nil, err
	}
	var rules []policy.Rule
	if bundle != nil {
		rules = bundle.Rules
	}
	return policy.BeginExplanation(func(event policy.Explanation) {
		ui.Writef("policy: %s %s: %s\n", event.Kind, event.Rule, event.Status)
	}, rules), nil
}
