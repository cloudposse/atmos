package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// VerifyPlanfile verifies that a stored planfile matches the current state before applying.
// It generates a fresh plan and compares it to the stored one using the plan-diff infrastructure.
// Returns an error wrapping ErrPlanVerificationFailed if drift is detected.
func VerifyPlanfile(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.VerifyPlanfile")()

	// Validate that a planfile is specified.
	if info.PlanFile == "" {
		return fmt.Errorf("%w: --verify-plan requires a planfile (use --from-plan or --planfile)", errUtils.ErrPlanVerificationFailed)
	}

	// Validate stored planfile exists on disk.
	if _, err := os.Stat(info.PlanFile); os.IsNotExist(err) {
		return fmt.Errorf("%w: planfile does not exist: %s", errUtils.ErrPlanVerificationFailed, info.PlanFile)
	}

	// Initialize CLI config with stack processing to resolve component metadata.
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config for plan verification: %w", err)
	}

	// Process stacks to resolve component metadata (metadata.component, component_folder_prefix).
	processedInfo, err := ProcessStacks(&atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return fmt.Errorf("error processing stacks for plan verification: %w", err)
	}

	// Resolve the component path.
	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, processedInfo.ComponentFolderPrefix, processedInfo.FinalComponent)

	// Check if workdir is enabled (source + workdir or workdir only).
	if workdirPath, ok := processedInfo.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		componentPath = workdirPath
	}

	// Create a temporary directory for the fresh plan.
	tmpDir, err := os.MkdirTemp("", "atmos-verify-plan")
	if err != nil {
		return fmt.Errorf("error creating temporary directory for plan verification: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Get JSON representation of the stored plan.
	storedPlanJSON, err := getTerraformPlanJSON(&atmosConfig, &processedInfo, componentPath, info.PlanFile)
	if err != nil {
		return fmt.Errorf("error getting JSON for stored plan: %w", err)
	}

	// Generate a fresh plan.
	freshPlanFile, err := generateNewPlanFile(&atmosConfig, &processedInfo, componentPath, tmpDir)
	if err != nil {
		return fmt.Errorf("error generating fresh plan for verification: %w", err)
	}

	// Get JSON representation of the fresh plan.
	freshPlanJSON, err := getTerraformPlanJSON(&atmosConfig, &processedInfo, componentPath, freshPlanFile)
	if err != nil {
		return fmt.Errorf("error getting JSON for fresh plan: %w", err)
	}

	// Parse both JSONs.
	var storedPlan, freshPlan map[string]interface{}
	if err := json.Unmarshal([]byte(storedPlanJSON), &storedPlan); err != nil {
		return fmt.Errorf("error parsing stored plan JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(freshPlanJSON), &freshPlan); err != nil {
		return fmt.Errorf("error parsing fresh plan JSON: %w", err)
	}

	// Sort maps for deterministic comparison.
	storedPlan = sortMapKeys(storedPlan)
	freshPlan = sortMapKeys(freshPlan)

	// Generate diff.
	diff, hasDiff := generatePlanDiff(storedPlan, freshPlan)

	if hasDiff {
		return fmt.Errorf("%w:\n%s", errUtils.ErrPlanVerificationFailed, diff)
	}

	log.Info("Plan verification passed: stored plan matches current state")
	return nil
}
