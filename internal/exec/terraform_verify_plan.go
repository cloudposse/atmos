package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// VerifyPlanfile verifies that a stored planfile matches the current state before applying.
// It generates a fresh plan and compares it to the stored one using the plan-diff infrastructure.
// The storedPlanFile parameter is the path to the stored planfile to verify against.
// On a match (or when mode is warn) info.PlanFile is set to the fresh planfile path and
// info.UseTerraformPlan is set to true so the subsequent apply uses the verified fresh plan.
// The stored planfile is cleaned up. When drift is detected and mode is fail, it returns an
// error wrapping ErrPlanVerificationFailed; when mode is warn, it logs the drift and proceeds.
func VerifyPlanfile(info *schema.ConfigAndStacksInfo, storedPlanFile string, mode schema.PlanfileVerifyMode) error {
	defer perf.Track(nil, "exec.VerifyPlanfile")()

	// Validate stored planfile exists on disk.
	if _, err := os.Stat(storedPlanFile); os.IsNotExist(err) {
		return fmt.Errorf("%w: stored planfile does not exist: %s", errUtils.ErrPlanVerificationFailed, storedPlanFile)
	}

	vc, err := resolveVerificationContext(info)
	if err != nil {
		return err
	}

	plans, err := loadStoredAndFreshPlans(&vc.atmosConfig, &vc.processedInfo, vc.componentPath, storedPlanFile)
	if err != nil {
		os.Remove(storedPlanFile)
		return err
	}

	// Generate diff. The stored planfile has served its purpose either way.
	diff, hasDiff := generatePlanDiff(sortMapKeys(plans.stored), sortMapKeys(plans.fresh))
	os.Remove(storedPlanFile)

	return finalizeVerification(info, plans.freshPlanFile, diff, hasDiff, mode)
}

// verificationContext holds the resolved config, processed component info, and
// component working directory needed to generate the fresh plan for comparison.
type verificationContext struct {
	atmosConfig   schema.AtmosConfiguration
	processedInfo schema.ConfigAndStacksInfo
	componentPath string
}

// resolveVerificationContext initializes the CLI config, processes stacks, and
// resolves the component working directory used to generate the fresh plan.
func resolveVerificationContext(info *schema.ConfigAndStacksInfo) (verificationContext, error) {
	defer perf.Track(nil, "exec.resolveVerificationContext")()

	vc := verificationContext{}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return vc, fmt.Errorf("failed to initialize CLI config for plan verification: %w", err)
	}
	vc.atmosConfig = atmosConfig

	// Process stacks to resolve component metadata (metadata.component, component_folder_prefix).
	processedInfo, err := ProcessStacks(&atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return vc, fmt.Errorf("error processing stacks for plan verification: %w", err)
	}
	vc.processedInfo = processedInfo

	vc.componentPath = filepath.Join(atmosConfig.TerraformDirAbsolutePath, processedInfo.ComponentFolderPrefix, processedInfo.FinalComponent)

	// For workdir-enabled components, compute the workdir path from first principles
	// because ProcessStacks builds a fresh ComponentSection that does not include
	// WorkdirPathKey (only set by AutoProvisionSource at execution time).
	if provWorkdir.IsWorkdirEnabled(processedInfo.ComponentSection) {
		candidate, exists, resolveErr := component.BuildAndResolveWorkdirPath(&atmosConfig, &processedInfo, cfg.TerraformComponentType)
		if resolveErr != nil {
			return vc, resolveErr
		}
		if exists {
			vc.componentPath = candidate
		}
	}

	return vc, nil
}

// comparisonPlans holds the parsed stored and fresh plan JSON (and the fresh
// plan file path) used for the drift comparison.
type comparisonPlans struct {
	stored        map[string]interface{}
	fresh         map[string]interface{}
	freshPlanFile string
}

// loadStoredAndFreshPlans returns the parsed JSON of the stored plan and of a
// freshly generated plan (plus the fresh plan file path) for comparison.
func loadStoredAndFreshPlans(atmosConfig *schema.AtmosConfiguration, processedInfo *schema.ConfigAndStacksInfo, componentPath, storedPlanFile string) (comparisonPlans, error) {
	defer perf.Track(nil, "exec.loadStoredAndFreshPlans")()

	var plans comparisonPlans

	storedPlanJSON, err := getTerraformPlanJSON(atmosConfig, processedInfo, componentPath, storedPlanFile)
	if err != nil {
		return plans, fmt.Errorf("error getting JSON for stored plan: %w", err)
	}

	// Generate a fresh plan at the canonical planfile path in the component dir,
	// where terraform expects it, rather than in a temp dir.
	freshPlanPath := constructTerraformComponentPlanfilePath(atmosConfig, processedInfo)
	plans.freshPlanFile, err = generateNewPlanFile(atmosConfig, processedInfo, componentPath, filepath.Dir(freshPlanPath))
	if err != nil {
		return plans, fmt.Errorf("error generating fresh plan for verification: %w", err)
	}

	freshPlanJSON, err := getTerraformPlanJSON(atmosConfig, processedInfo, componentPath, plans.freshPlanFile)
	if err != nil {
		return plans, fmt.Errorf("error getting JSON for fresh plan: %w", err)
	}

	if err := json.Unmarshal([]byte(storedPlanJSON), &plans.stored); err != nil {
		return plans, fmt.Errorf("error parsing stored plan JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(freshPlanJSON), &plans.fresh); err != nil {
		return plans, fmt.Errorf("error parsing fresh plan JSON: %w", err)
	}

	return plans, nil
}

// finalizeVerification applies the verification verdict. On drift with mode fail
// it returns ErrPlanVerificationFailed; otherwise (match, or drift under warn) it
// selects the freshly generated plan for the subsequent apply, logging a warning
// when proceeding despite drift.
func finalizeVerification(info *schema.ConfigAndStacksInfo, freshPlanFile, diff string, hasDiff bool, mode schema.PlanfileVerifyMode) error {
	if hasDiff && mode == schema.PlanfileVerifyFail {
		return fmt.Errorf("%w:\n%s", errUtils.ErrPlanVerificationFailed, diff)
	}

	// Match, or warn-on-drift: apply the freshly generated plan (current state).
	info.PlanFile = freshPlanFile
	info.UseTerraformPlan = true

	if hasDiff {
		// mode == warn: surface the drift but do not block the deploy.
		log.Warn("Plan verification detected drift; proceeding because verify mode is 'warn'", "diff", diff)
	} else {
		log.Info("Plan verification passed: stored plan matches current state")
	}
	return nil
}
