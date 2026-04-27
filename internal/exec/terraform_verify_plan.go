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
// The storedPlanFile parameter is the path to the stored planfile to verify against.
// On success, info.PlanFile is set to the fresh planfile path and info.UseTerraformPlan is set to true
// so the subsequent apply uses the verified fresh plan. The stored planfile is cleaned up.
// Returns an error wrapping ErrPlanVerificationFailed if drift is detected.
func VerifyPlanfile(info *schema.ConfigAndStacksInfo, storedPlanFile string) error {
	defer perf.Track(nil, "exec.VerifyPlanfile")()

	// Validate stored planfile exists on disk.
	if _, err := os.Stat(storedPlanFile); os.IsNotExist(err) {
		return fmt.Errorf("%w: stored planfile does not exist: %s", errUtils.ErrPlanVerificationFailed, storedPlanFile)
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

	// For workdir-enabled components, compute the workdir path from first principles.
	// ProcessStacks builds a fresh ComponentSection that does not include WorkdirPathKey
	// (only set by AutoProvisionSource at execution time), so we derive the path directly.
	// applyMetadataComponentSubpath handles the metadata.component subdir within the workdir.
	if provWorkdir.IsWorkdirEnabled(processedInfo.ComponentSection) {
		workdirRoot := provWorkdir.BuildPath(
			atmosConfig.BasePath,
			cfg.TerraformComponentType,
			processedInfo.FinalComponent,
			processedInfo.Stack,
			processedInfo.ComponentSection,
		)
		candidate := applyMetadataComponentSubpath(processedInfo.BaseComponentPath, workdirRoot)
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			componentPath = candidate
		}
	}

	// Get JSON representation of the stored plan.
	storedPlanJSON, err := getTerraformPlanJSON(&atmosConfig, &processedInfo, componentPath, storedPlanFile)
	if err != nil {
		return fmt.Errorf("error getting JSON for stored plan: %w", err)
	}

	// Generate a fresh plan at the canonical planfile path in the component dir.
	// This places the fresh plan where terraform expects it, rather than in a temp dir.
	freshPlanPath := constructTerraformComponentPlanfilePath(&atmosConfig, &processedInfo)
	freshPlanDir := filepath.Dir(freshPlanPath)

	freshPlanFile, err := generateNewPlanFile(&atmosConfig, &processedInfo, componentPath, freshPlanDir)
	if err != nil {
		os.Remove(storedPlanFile)
		return fmt.Errorf("error generating fresh plan for verification: %w", err)
	}

	// Get JSON representation of the fresh plan.
	freshPlanJSON, err := getTerraformPlanJSON(&atmosConfig, &processedInfo, componentPath, freshPlanFile)
	if err != nil {
		os.Remove(storedPlanFile)
		return fmt.Errorf("error getting JSON for fresh plan: %w", err)
	}

	// Parse both JSONs.
	var storedPlan, freshPlan map[string]interface{}
	if err := json.Unmarshal([]byte(storedPlanJSON), &storedPlan); err != nil {
		os.Remove(storedPlanFile)
		return fmt.Errorf("error parsing stored plan JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(freshPlanJSON), &freshPlan); err != nil {
		os.Remove(storedPlanFile)
		return fmt.Errorf("error parsing fresh plan JSON: %w", err)
	}

	// Sort maps for deterministic comparison.
	storedPlan = sortMapKeys(storedPlan)
	freshPlan = sortMapKeys(freshPlan)

	// Generate diff.
	diff, hasDiff := generatePlanDiff(storedPlan, freshPlan)

	if hasDiff {
		os.Remove(storedPlanFile)
		return fmt.Errorf("%w:\n%s", errUtils.ErrPlanVerificationFailed, diff)
	}

	// Verification passed — set info to use fresh plan for apply.
	info.PlanFile = freshPlanFile
	info.UseTerraformPlan = true
	os.Remove(storedPlanFile)

	log.Info("Plan verification passed: stored plan matches current state")
	return nil
}
