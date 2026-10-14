package exec

import (
	"context"
	"fmt"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/planfile"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Load once before configuration execution; component env cannot change the source.
// A protected caller owns the environment/directory. This is not trusted-tier discovery.
func loadTerraformPlanPolicy(info *schema.ConfigAndStacksInfo) (*planfile.Guard, error) {
	if info.NeedHelp {
		return nil, nil
	}
	bundle, err := admission.LoadBundle()
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		return nil, nil
	}
	if err := validateTerraformPolicyCommand(info); err != nil {
		return nil, err
	}
	if !governedTerraformCommand(info.SubCommand) {
		return nil, nil
	}
	rules := bundle.RulesFor(policy.TerraformPlan)
	if len(rules) == 0 {
		return nil, nil
	}
	gate, err := newTerraformPolicyGuard(rules)
	if err != nil {
		return nil, err
	}
	return gate, nil
}

// bindTerraformPolicy attaches loaded definitions after inherited instance metadata
// is resolved, before starting component tools. Out-of-scope instances retain their
// normal command behavior; a missing required subject fails closed.
func bindTerraformPolicy(gate *planfile.Guard, info *schema.ConfigAndStacksInfo, delimiters ...string) (*planfile.Guard, error) {
	if gate == nil {
		return nil, nil
	}
	subject, err := policy.NewSubject(policy.ObjectIdentity{
		Name: info.ComponentFromArg, Stack: info.Stack, ComponentType: "terraform",
	}, info.ComponentSection[cfg.MetadataSectionName], delimiters...)
	if err != nil {
		return nil, err
	}
	var selected []policy.Rule
	for i := range gate.Rules {
		rule := &gate.Rules[i]
		matches, err := rule.Match.Matches(subject)
		if err != nil {
			return nil, err
		}
		if matches {
			selected = append(selected, *rule)
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	gate.Rules, gate.Subject = selected, subject
	return gate, validateTerraformPolicyOperation(info, gate)
}

func newTerraformPolicyGuard(rules []policy.Rule) (*planfile.Guard, error) {
	engine := policy.NewEngine()
	gate := &planfile.Guard{Engine: engine, Rules: rules, Show: process.NewDefaultRunner(), ResolveApprovals: ci.ResolvePolicyApprovals}
	snapshot, err := ci.CollectPolicySnapshot(context.Background())
	if err != nil {
		return nil, err
	}
	gate.CI = snapshot
	return gate, nil
}

func governedTerraformCommand(command string) bool {
	switch command {
	case "plan", subcommandApply, subcommandDeploy, "destroy":
		return true
	default:
		return false
	}
}

func validateTerraformPolicyOperation(info *schema.ConfigAndStacksInfo, gate *planfile.Guard) error {
	if gate == nil {
		return nil
	}
	if info.SubCommand == "destroy" ||
		((info.SubCommand == subcommandApply || info.SubCommand == subcommandDeploy) && !info.UseTerraformPlan) {
		return fmt.Errorf("%w: plan policies require an explicit saved plan; run terraform plan (with -destroy for deletion), "+
			"then terraform apply --from-plan or --planfile", errUtils.ErrPolicyPlan)
	}
	return nil
}

func terraformPolicyInvocation(
	atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, args []string, dir string, opts []ShellCommandOption,
) (context.Context, planfile.Invocation) {
	var config shellCommandConfig
	for _, option := range opts {
		option(&config)
	}
	ctx := config.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	env := config.processEnv
	if env == nil {
		env = os.Environ()
	}
	env = append([]string{}, env...)
	env = envpkg.MergeGlobalEnv(env, atmosConfig.Env)
	env = append(env, info.ComponentEnvList...)
	// Do not let Terraform's own debug logging persist the sensitive extraction.
	env = append(env, "TF_LOG=", "TF_LOG_CORE=", "TF_LOG_PROVIDER=", "TF_LOG_PATH=")
	return ctx, planfile.Invocation{Command: info.Command, Args: args, Dir: dir, Env: env}
}

// executeGovernedTerraformCommand keeps the policy gate outside CI exit-code
// mapping and best-effort uploads, while using the existing executor for the main operation.
func executeGovernedTerraformCommand( //nolint:revive // argument-limit: mirrors the existing main command executor plus its gate.
	atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, args []string, componentPath string,
	uploadStatusFlag bool, gate *planfile.Guard, opts ...ShellCommandOption,
) error {
	if gate == nil {
		return executeMainTerraformCommand(atmosConfig, info, args, componentPath, uploadStatusFlag, opts...)
	}
	if err := validateTerraformPolicyOperation(info, gate); err != nil {
		return err
	}
	if info.DryRun {
		log.Warn("Dry-run: saved-plan policies were NOT evaluated", "rules", len(gate.Rules))
		return executeMainTerraformCommand(atmosConfig, info, args, componentPath, uploadStatusFlag, opts...)
	}
	ctx, invocation := terraformPolicyInvocation(atmosConfig, info, args, componentPath, opts)
	result, gateErr := gate.Execute(ctx, &invocation, func(privateArgs []string) process.Result {
		err := runMainTerraformCommand(atmosConfig, info, privateArgs, componentPath, opts...)
		return process.Result{Err: err, ExitCode: resolveExitCode(err), Started: true, Canceled: ctx.Err() != nil}
	})
	if gateErr != nil {
		return gateErr
	}
	return finishMainTerraformCommand(atmosConfig, info, result.Err, uploadStatusFlag)
}

// Keep the loader's command scope exact, even if a caller passes whitespace.
func validateTerraformPolicyCommand(info *schema.ConfigAndStacksInfo) error {
	fields := strings.Fields(info.SubCommand)
	if len(fields) > 1 && governedTerraformCommand(fields[0]) {
		return fmt.Errorf("%w: Terraform command arguments must be passed separately", errUtils.ErrPolicyPlan)
	}
	return nil
}
