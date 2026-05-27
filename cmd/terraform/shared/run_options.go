package shared

import (
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RunOptions contains shared flags from the terraform parser.
type RunOptions struct {
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

	// Status upload flag.
	UploadStatus bool
}

// ParseRunOptions parses shared terraform flags from Viper.
func ParseRunOptions(v *viper.Viper) *RunOptions {
	defer perf.Track(nil, "terraform.shared.ParseRunOptions")()

	return &RunOptions{
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
		UploadStatus:            v.GetBool("upload-status"),
	}
}

// ApplyRunOptions transfers parsed options to the info struct.
func ApplyRunOptions(info *schema.ConfigAndStacksInfo, opts *RunOptions) {
	info.ProcessTemplates = opts.ProcessTemplates
	info.ProcessFunctions = opts.ProcessFunctions
	info.Skip = opts.Skip
	info.Components = opts.Components
	info.DryRun = opts.DryRun
	info.SkipInit = opts.SkipInit
	info.UploadStatus = opts.UploadStatus
	info.All = opts.All
	info.Affected = opts.Affected
	info.Query = opts.Query

	// Backend execution flags only apply if set via CLI.
	if opts.AutoGenerateBackendFile != "" {
		info.AutoGenerateBackendFile = opts.AutoGenerateBackendFile
	}
	if opts.InitRunReconfigure != "" {
		info.InitRunReconfigure = opts.InitRunReconfigure
	}
	if opts.InitPassVars {
		info.InitPassVars = "true"
	}

	if opts.PlanFile != "" {
		info.PlanFile = opts.PlanFile
		info.UseTerraformPlan = true
	}
	if opts.PlanSkipPlanfile {
		info.PlanSkipPlanfile = "true"
	}
	if opts.DeployRunInit {
		info.DeployRunInit = "true"
	}
	info.VerifyPlan = opts.VerifyPlan
}
