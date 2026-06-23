package terraform

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	terraformFailureModeFailFast  = "fail-fast"
	terraformFailureModeKeepGoing = "keep-going"
	terraformPlanLogOrderStream   = "stream"
	terraformPlanLogOrderGrouped  = "grouped"
)

// TerraformRunOptions contains shared flags from terraformParser.
// Used by simple subcommands that only need the base terraform flags.
type TerraformRunOptions struct {
	// Processing flags.
	ProcessTemplates bool
	ProcessFunctions bool
	Skip             []string

	// Execution flags.
	DryRun       bool
	SkipInit     bool
	InitPassVars bool

	// Backend execution flags.
	AutoGenerateBackendFile string
	InitRunReconfigure      string

	// Plan/Apply/Deploy specific flags.
	PlanFile         string
	PlanSkipPlanfile bool
	DeployRunInit    bool
	// VerifyPlan is the tri-state planfile-verify override from --verify-plan:
	// nil when the flag was not set, &true for --verify-plan(=true), &false for
	// --verify-plan=false. nil falls back to config / the CI default.
	VerifyPlan *bool

	// Multi-component flags.
	Query      string
	Components []string
	All        bool
	Affected   bool

	// Graph-backed Terraform concurrency.
	MaxConcurrency    int
	FailureMode       string
	PlanLogOrder      string
	PlanHide          []string
	PlanHideNoChanges bool
	PlanSummaryFile   string

	// Status upload flag.
	UploadStatus bool
}

// VerifyPlanCLIOverride returns the explicit planfile-verify mode requested via
// the --verify-plan flag: fail for --verify-plan(=true), off for --verify-plan=false,
// empty when the flag was not set (defer to config / the CI default).
func (o *TerraformRunOptions) VerifyPlanCLIOverride() schema.PlanfileVerifyMode {
	if o.VerifyPlan == nil {
		return ""
	}
	if *o.VerifyPlan {
		return schema.PlanfileVerifyFail
	}
	return schema.PlanfileVerifyOff
}

// ParseTerraformRunOptions parses and validates shared terraform flags from Viper.
func ParseTerraformRunOptions(v *viper.Viper) (*TerraformRunOptions, error) {
	defer perf.Track(nil, "terraform.ParseTerraformRunOptions")()

	opts := &TerraformRunOptions{
		ProcessTemplates:        v.GetBool("process-templates"),
		ProcessFunctions:        v.GetBool("process-functions"),
		Skip:                    v.GetStringSlice("skip"),
		DryRun:                  v.GetBool("dry-run"),
		SkipInit:                v.GetBool("skip-init"),
		InitPassVars:            v.GetBool("init-pass-vars"),
		AutoGenerateBackendFile: v.GetString("auto-generate-backend-file"),
		InitRunReconfigure:      v.GetString("init-run-reconfigure"),
		PlanFile:                v.GetString("planfile"),
		PlanSkipPlanfile:        v.GetBool("skip-planfile"),
		DeployRunInit:           v.GetBool("deploy-run-init"),
		Query:                   v.GetString("query"),
		Components:              v.GetStringSlice("components"),
		All:                     v.GetBool("all"),
		Affected:                v.GetBool("affected"),
		MaxConcurrency:          v.GetInt("max-concurrency"),
		FailureMode:             v.GetString("failure-mode"),
		PlanLogOrder:            v.GetString("log-order"),
		PlanHide:                v.GetStringSlice("hide"),
		PlanHideNoChanges:       terraformPlanHideContains(v.GetStringSlice("hide"), "no-changes"),
		PlanSummaryFile:         v.GetString("execution-summary-file"),
		UploadStatus:            v.GetBool("upload-status"),
	}
	// Tri-state --verify-plan: only record a value when the flag (or its env var)
	// was explicitly set, so an unset flag defers to config / the CI default.
	if v.IsSet("verify-plan") {
		verifyPlan := v.GetBool("verify-plan")
		opts.VerifyPlan = &verifyPlan
	}
	if err := validateTerraformRunOptions(opts); err != nil {
		return nil, err
	}
	return opts, nil
}

func validateTerraformRunOptions(opts *TerraformRunOptions) error {
	if opts == nil {
		return nil
	}

	if mode := strings.ToLower(strings.TrimSpace(opts.FailureMode)); mode != "" {
		switch mode {
		case terraformFailureModeFailFast, terraformFailureModeKeepGoing:
			opts.FailureMode = mode
		default:
			return fmt.Errorf("invalid --failure-mode %q: supported values are %q, %q", opts.FailureMode, terraformFailureModeFailFast, terraformFailureModeKeepGoing)
		}
	}

	if logOrder := strings.ToLower(strings.TrimSpace(opts.PlanLogOrder)); logOrder != "" {
		switch logOrder {
		case terraformPlanLogOrderStream, terraformPlanLogOrderGrouped:
			opts.PlanLogOrder = logOrder
		default:
			return fmt.Errorf("invalid --log-order %q: supported values are %q, %q", opts.PlanLogOrder, terraformPlanLogOrderStream, terraformPlanLogOrderGrouped)
		}
	}
	return nil
}

func terraformPlanHideContains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}
