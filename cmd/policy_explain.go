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
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--explain" {
			enabled = true
		}
		if strings.HasPrefix(arg, "--explain=") {
			enabled, _ = strconv.ParseBool(strings.TrimPrefix(arg, "--explain="))
		}
	}
	return enabled
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
