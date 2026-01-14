package terraform

import (
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

	// Multi-component flags.
	Query      string
	Components []string
	All        bool
	Affected   bool
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
		Query:                   v.GetString("query"),
		Components:              v.GetStringSlice("components"),
		All:                     v.GetBool("all"),
		Affected:                v.GetBool("affected"),
	}
}
