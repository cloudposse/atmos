package terraform

import (
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
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
	VerifyPlan       bool

	// Multi-component flags.
	Query      string
	Components []string
	All        bool
	Affected   bool

	// Graph-backed Terraform concurrency.
	MaxConcurrency    int
	FailFast          bool
	KeepGoing         bool
	PlanLogOrder      string
	PlanHide          []string
	PlanHideNoChanges bool
	PlanSummaryFile   string

	// Status upload flag.
	UploadStatus bool
}

// ParseTerraformRunOptions parses shared terraform flags from Viper.
func ParseTerraformRunOptions(v *viper.Viper) *TerraformRunOptions {
	defer perf.Track(nil, "terraform.ParseTerraformRunOptions")()

	return &TerraformRunOptions{
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
		VerifyPlan:              v.GetBool("verify-plan"),
		Query:                   v.GetString("query"),
		Components:              v.GetStringSlice("components"),
		All:                     v.GetBool("all"),
		Affected:                v.GetBool("affected"),
		MaxConcurrency:          v.GetInt("max-concurrency"),
		FailFast:                v.GetBool("fail-fast"),
		KeepGoing:               v.GetBool("keep-going"),
		PlanLogOrder:            v.GetString("log-order"),
		PlanHide:                v.GetStringSlice("hide"),
		PlanHideNoChanges:       terraformPlanHideContains(v.GetStringSlice("hide"), "no-changes"),
		PlanSummaryFile:         v.GetString("execution-summary-file"),
		UploadStatus:            v.GetBool("upload-status"),
	}
}

func terraformPlanHideContains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}
