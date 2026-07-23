package shared

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

const (
	TerraformFailureModeFailFast  = "fail-fast"
	TerraformFailureModeKeepGoing = "keep-going"
	TerraformLogOrderStream       = "stream"
	TerraformLogOrderGrouped      = "grouped"
)

// RunOptions contains shared flags from the terraform parser.
type RunOptions struct {
	// Processing flags.
	ProcessTemplates bool
	ProcessFunctions bool
	UseMocks         bool
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
	Tags       []string
	Labels     map[string]string
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

// ParseRunOptions parses and validates shared terraform flags from Viper.
func ParseRunOptions(v *viper.Viper) (*RunOptions, error) {
	defer perf.Track(nil, "terraform.shared.ParseRunOptions")()

	opts := &RunOptions{
		ProcessTemplates:        v.GetBool("process-templates"),
		ProcessFunctions:        v.GetBool("process-functions"),
		UseMocks:                v.GetBool("use-mocks"),
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
		Tags:                    v.GetStringSlice("tags"),
		All:                     v.GetBool("all"),
		Affected:                v.GetBool("affected"),
		MaxConcurrency:          v.GetInt("max-concurrency"),
		FailureMode:             v.GetString("failure-mode"),
		LogOrder:                v.GetString("log-order"),
		PlanHide:                v.GetStringSlice("hide"),
		PlanHideNoChanges:       TerraformPlanHideContains(v.GetStringSlice("hide"), "no-changes"),
		PlanSummaryFile:         v.GetString("execution-summary-file"),
		UploadStatus:            v.GetBool("upload-status"),
	}

	labels, err := tags.ParseLabelsFlag(v.GetString("labels"))
	if err != nil {
		return nil, err
	}
	opts.Labels = labels

	if err := ValidateRunOptions(opts); err != nil {
		return nil, err
	}
	return opts, nil
}

// ValidateRunOptions normalizes and validates Terraform run options.
func ValidateRunOptions(opts *RunOptions) error {
	if opts == nil {
		return nil
	}

	if mode := strings.ToLower(strings.TrimSpace(opts.FailureMode)); mode != "" {
		switch mode {
		case TerraformFailureModeFailFast, TerraformFailureModeKeepGoing:
			opts.FailureMode = mode
		default:
			return fmt.Errorf("%w: invalid --failure-mode %q: supported values are %q, %q", errUtils.ErrInvalidFlagValue, opts.FailureMode, TerraformFailureModeFailFast, TerraformFailureModeKeepGoing)
		}
	}

	if logOrder := strings.ToLower(strings.TrimSpace(opts.LogOrder)); logOrder != "" {
		switch logOrder {
		case TerraformLogOrderStream, TerraformLogOrderGrouped:
			opts.LogOrder = logOrder
		default:
			return fmt.Errorf("%w: invalid --log-order %q: supported values are %q, %q", errUtils.ErrInvalidFlagValue, opts.LogOrder, TerraformLogOrderStream, TerraformLogOrderGrouped)
		}
	}
	return nil
}

// TerraformPlanHideContains reports whether values contains target, ignoring case and whitespace.
func TerraformPlanHideContains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

// ApplyRunOptions transfers parsed options to the info struct.
func ApplyRunOptions(info *schema.ConfigAndStacksInfo, opts *RunOptions) {
	info.ProcessTemplates = opts.ProcessTemplates
	info.ProcessFunctions = opts.ProcessFunctions
	info.UseMocks = opts.UseMocks
	info.Skip = opts.Skip
	info.Components = opts.Components
	info.Tags = opts.Tags
	info.Labels = opts.Labels
	info.DryRun = opts.DryRun
	info.SkipInit = opts.SkipInit
	info.UploadStatus = opts.UploadStatus
	info.All = opts.All
	info.Affected = opts.Affected
	info.Query = opts.Query
	info.MaxConcurrency = opts.MaxConcurrency
	info.TerraformFailureMode = opts.FailureMode
	info.FailFast = opts.FailureMode == TerraformFailureModeFailFast
	info.KeepGoing = opts.FailureMode == TerraformFailureModeKeepGoing
	info.TerraformLogOrder = opts.LogOrder
	info.TerraformPlanHide = opts.PlanHide
	info.TerraformPlanHideNoChanges = opts.PlanHideNoChanges
	info.TerraformPlanSummaryFile = opts.PlanSummaryFile

	if len(opts.AppendArgs) > 0 {
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, opts.AppendArgs...)
	}

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
}
