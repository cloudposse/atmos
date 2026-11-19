package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// prepareNewPlanFile handles the new plan file (generates one if not provided).
func (s *Operations) prepareNewPlanFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts PlanFileOptions) (string, error) {
	// If no new plan file is specified, generate one
	if opts.NewPlanFile == "" {
		var err error
		opts.NewPlanFile, err = s.GenerateNewPlanFile(atmosConfig, info, opts.ComponentPath, opts.TmpDir)
		if err != nil {
			return "", errors.Wrap(err, "error generating new plan file")
		}
	} else if !filepath.IsAbs(opts.NewPlanFile) {
		// If the path is relative, make it absolute based on the component directory
		opts.NewPlanFile = filepath.Join(opts.ComponentPath, opts.NewPlanFile)
	}

	// Make sure the new plan file exists
	if _, err := os.Stat(opts.NewPlanFile); os.IsNotExist(err) {
		return "", errors.Errorf("new plan file '%s' does not exist", opts.NewPlanFile)
	}

	return opts.NewPlanFile, nil
}

// FilterPlanDiffFlags filters out --orig and --new flags from arguments.
func FilterPlanDiffFlags(args []string) []string {
	defer perf.Track(nil, "terraform.FilterPlanDiffFlags")()

	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// Skip --orig and --new flags and their values
		if arg == "--orig" || arg == "--new" {
			// Skip the value too if it exists
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}

		if strings.HasPrefix(arg, "--orig=") || strings.HasPrefix(arg, "--new=") {
			continue
		}

		filteredArgs = append(filteredArgs, arg)
	}
	return filteredArgs
}

// GenerateNewPlanFile generates a new plan file by running terraform plan.
func (s *Operations) GenerateNewPlanFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, tmpDir string) (string, error) {
	defer perf.Track(atmosConfig, "terraform.Operations.GenerateNewPlanFile")()

	// Create a temporary file for the new plan
	newPlanFile := filepath.Join(tmpDir, "new.plan")

	// Run terraform init before plan
	if err := s.runTerraformInit(atmosConfig, componentPath, info); err != nil {
		return "", err
	}

	// Create a new info object for the plan command
	planInfo := *info
	planInfo.SubCommand = "plan"

	// Process templates and Atmos YAML functions in the plan command.
	planInfo.ProcessTemplates = true
	planInfo.ProcessFunctions = true

	// Filter out --orig and --new flags from AdditionalArgsAndFlags
	planArgs := FilterPlanDiffFlags(info.AdditionalArgsAndFlags)

	// Add -out flag to specify the output plan file
	planArgs = append(planArgs, "-out="+newPlanFile)

	// Update the AdditionalArgsAndFlags with our filtered and augmented args
	planInfo.AdditionalArgsAndFlags = planArgs

	// Execute the plan command using the standard Atmos terraform execution
	err := s.host.ExecuteTerraform(planInfo)
	if err != nil {
		// If the error is ErrPlanHasDiff, we want to propagate that error
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			return "", err
		}
		return "", fmt.Errorf("error running terraform plan: %w", err)
	}

	return newPlanFile, nil
}
