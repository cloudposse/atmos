package terraform

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	terraformFailureModeFailFast  = "fail-fast"
	terraformFailureModeKeepGoing = "keep-going"
	terraformLogOrderStream       = "stream"
	terraformLogOrderGrouped      = "grouped"
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

	// Graph-backed Terraform concurrency.
	MaxConcurrency    int
	FailureMode       string
	LogOrder          string
	PlanHide          []string
	PlanHideNoChanges bool
	PlanSummaryFile   string

	// Status upload flag.
	UploadStatus bool

	// AppendArgs are extra terraform pass-through flags injected by the caller
	// (e.g. `-json` for `terraform test` in CI). They are appended to
	// info.AdditionalArgsAndFlags so they reach the terraform command directly,
	// bypassing Cobra positional-arg parsing.
	AppendArgs []string
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
		LogOrder:                v.GetString("log-order"),
		PlanHide:                v.GetStringSlice("hide"),
		PlanHideNoChanges:       terraformPlanHideContains(v.GetStringSlice("hide"), "no-changes"),
		PlanSummaryFile:         v.GetString("execution-summary-file"),
		UploadStatus:            v.GetBool("upload-status"),
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
			return fmt.Errorf("%w: --failure-mode %q (valid values: %s, %s)",
				errUtils.ErrInvalidFlagValue, opts.FailureMode, terraformFailureModeFailFast, terraformFailureModeKeepGoing)
		}
	}

	if logOrder := strings.ToLower(strings.TrimSpace(opts.LogOrder)); logOrder != "" {
		switch logOrder {
		case terraformLogOrderStream, terraformLogOrderGrouped:
			opts.LogOrder = logOrder
		default:
			return fmt.Errorf("%w: --log-order %q (valid values: %s, %s)",
				errUtils.ErrInvalidFlagValue, opts.LogOrder, terraformLogOrderStream, terraformLogOrderGrouped)
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
