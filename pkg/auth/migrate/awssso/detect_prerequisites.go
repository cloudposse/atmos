// Package awssso implements the AWS SSO migration steps.
package awssso

import (
	"context"
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/migrate"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DetectPrerequisites is a gate step that validates migration preconditions.
// It checks whether the repository uses aws-teams or aws-team-roles components,
// which indicate the SSO migration does not apply.
type DetectPrerequisites struct {
	migCtx *migrate.MigrationContext
	fs     migrate.FileSystem
}

// NewDetectPrerequisites creates a new prerequisites detection step.
func NewDetectPrerequisites(migCtx *migrate.MigrationContext, fs migrate.FileSystem) *DetectPrerequisites {
	return &DetectPrerequisites{migCtx: migCtx, fs: fs}
}

// Name returns the step identifier.
func (d *DetectPrerequisites) Name() string { return "detect-prerequisites" }

// Description returns a human-readable description of this step.
func (d *DetectPrerequisites) Description() string { return "Check migration prerequisites" }

// Detect checks if this step needs to run, is already complete, or does not apply.
// It returns StepNotApplicable if aws-teams or aws-team-roles configs are found,
// indicating the repository uses a teams-based auth model rather than SSO.
func (d *DetectPrerequisites) Detect(ctx context.Context) (migrate.StepStatus, error) {
	defer perf.Track(nil, "awssso.DetectPrerequisites.Detect")()

	base := d.migCtx.StacksBasePath
	log.Debug("Checking migration prerequisites", "stacks_base", base)

	// Check for aws-teams / aws-team-roles in known catalog locations.
	// filepath.Glob does not support recursive ** patterns, so we check
	// both the catalog root and one level of subdirectory.
	for _, name := range []string{"aws-teams.yaml", "aws-team-roles.yaml"} {
		// Check catalog root.
		directPath := filepath.Join(base, "catalog", name)
		if d.fs.Exists(directPath) {
			log.Debug("Found teams-based config — SSO migration not applicable", "file", directPath)
			return migrate.StepNotApplicable, nil
		}

		// Check one level of subdirectory under catalog.
		pattern := filepath.Join(base, "catalog", "*", name)
		matches, err := d.fs.Glob(pattern)
		if err != nil {
			return migrate.StepNotApplicable, fmt.Errorf("%w: %w", errUtils.ErrMigrationPrerequisitesNotMet, err)
		}
		if len(matches) > 0 {
			log.Debug("Found teams-based config — SSO migration not applicable", "matches", matches)
			return migrate.StepNotApplicable, nil
		}
	}

	log.Debug("No aws-teams/aws-team-roles found — migration may proceed")
	// No aws-teams found — migration may proceed.
	return migrate.StepNeeded, nil
}

// Plan is a no-op for the gate step.
func (d *DetectPrerequisites) Plan(_ context.Context) ([]migrate.Change, error) {
	return nil, nil
}

// Apply is a no-op for the gate step.
func (d *DetectPrerequisites) Apply(_ context.Context) error {
	return nil
}
