package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	terrerrors "github.com/cloudposse/atmos/pkg/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/pkg/errors"
)

// prepareNewPlanFile handles the new plan file (generates one if not provided).
func prepareNewPlanFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts PlanFileOptions) (string, error) {
	// If no new plan file is specified, generate one
	if opts.NewPlanFile == "" {
		var err error
		opts.NewPlanFile, err = generateNewPlanFile(atmosConfig, info, opts.ComponentPath, opts.TmpDir)
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

// generateNewPlanFile generates a new plan file by running terraform plan.
func generateNewPlanFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, tmpDir string) (string, error) {
	// Create a temporary file for the new plan
	newPlanFile := filepath.Join(tmpDir, "new.plan")

	// Run terraform init before plan
	if err := runTerraformInit(atmosConfig, componentPath, info); err != nil {
		return "", err
	}

	// Create a new info object for the plan command
	planInfo := *info
	planInfo.SubCommand = "plan"

	// Filter out --orig and --new flags from AdditionalArgsAndFlags
	var planArgs []string
	for i := 0; i < len(info.AdditionalArgsAndFlags); i++ {
		arg := info.AdditionalArgsAndFlags[i]

		// Skip --orig and --new flags and their values
		if arg == "--orig" || arg == "--new" {
			// Skip the value too if it exists
			if i+1 < len(info.AdditionalArgsAndFlags) && !strings.HasPrefix(info.AdditionalArgsAndFlags[i+1], "-") {
				i++
			}
			continue
		}

		if strings.HasPrefix(arg, "--orig=") || strings.HasPrefix(arg, "--new=") {
			continue
		}

		planArgs = append(planArgs, arg)
	}

	// Add -out flag to specify the output plan file
	planArgs = append(planArgs, "-out="+newPlanFile)

	// Update the AdditionalArgsAndFlags with our filtered and augmented args
	planInfo.AdditionalArgsAndFlags = planArgs

	// Execute the plan command using the standard Atmos terraform execution
	err := ExecuteTerraform(planInfo)
	if err != nil {
		// If the error is ErrPlanHasDiff, we want to propagate that error
		if errors.Is(err, terrerrors.ErrPlanHasDiff) {
			return "", err
		}
		return "", fmt.Errorf("error running terraform plan: %w", err)
	}

	return newPlanFile, nil
}
