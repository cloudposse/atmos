package cmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/ui"
)

// policyExplainRequested reads the boolean before project config can execute functions.
// The command line wins over ATMOS_EXPLAIN, which is how a nested Atmos process inherits the
// setting of its parent.
func policyExplainRequested(args []string) bool {
	enabled := false
	if value, found := os.LookupEnv(flags.ExplainEnvVar); found {
		enabled, _ = strconv.ParseBool(value)
	}
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

// exportPolicyExplanation publishes this invocation's explain setting in ATMOS_EXPLAIN so every
// nested `atmos` call (scripts, steps, custom commands, workflows) explains the same way. The
// returned function restores the previous value.
func exportPolicyExplanation(enabled bool) func() {
	previous, found := os.LookupEnv(flags.ExplainEnvVar)
	if (found && previous == strconv.FormatBool(enabled)) || (!found && !enabled) {
		return func() {}
	}
	_ = os.Setenv(flags.ExplainEnvVar, strconv.FormatBool(enabled))
	return func() {
		if found {
			_ = os.Setenv(flags.ExplainEnvVar, previous)
			return
		}
		_ = os.Unsetenv(flags.ExplainEnvVar)
	}
}

func beginPolicyExplanation(args []string) (func(), error) {
	enabled := policyExplainRequested(args)
	restoreEnv := exportPolicyExplanation(enabled)
	if !enabled {
		return restoreEnv, nil
	}
	bundle, err := admission.LoadBundle()
	if err != nil {
		restoreEnv()
		return nil, err
	}
	var rules []policy.Rule
	if bundle != nil {
		rules = bundle.Rules
	}
	restore := policy.BeginExplanation(func(event policy.Explanation) {
		ui.Writef("policy: %s %s: %s\n", event.Kind, event.Rule, event.Status)
	}, rules)
	return func() {
		restore()
		restoreEnv()
	}, nil
}
