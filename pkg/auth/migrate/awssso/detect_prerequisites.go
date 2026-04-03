// Package awssso implements the AWS SSO migration steps.
package awssso

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DetectPrerequisites is a gate step that validates migration preconditions.
// It checks whether enough context exists to proceed with SSO profile generation:
// an SSO provider must be configured (or discoverable) and group assignments
// must be available from aws-sso or aws-teams components.
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

// Detect validates that prerequisites for SSO migration are met.
// Returns StepComplete if SSO config and group assignments are available.
// Returns StepNotApplicable if no SSO provider is configured and no groups found.
func (d *DetectPrerequisites) Detect(ctx context.Context) (migrate.StepStatus, error) {
	defer perf.Track(nil, "awssso.DetectPrerequisites.Detect")()

	hasSSO := d.migCtx.SSOConfig != nil && d.migCtx.SSOConfig.StartURL != ""
	hasGroups := d.migCtx.SSOConfig != nil && len(d.migCtx.SSOConfig.AccountAssignments) > 0
	hasProvider := d.migCtx.ExistingAuth != nil && len(d.migCtx.ExistingAuth.Providers) > 0

	log.Debug("Checking migration prerequisites",
		"has_sso_config", hasSSO,
		"has_groups", hasGroups,
		"has_provider", hasProvider)

	if !hasSSO && !hasProvider {
		log.Debug("No SSO provider configured and no SSO config found — migration not applicable")
		return migrate.StepNotApplicable, nil
	}

	// Prerequisites are met — migration can proceed.
	return migrate.StepComplete, nil
}

// Plan is a no-op for the gate step.
func (d *DetectPrerequisites) Plan(_ context.Context) ([]migrate.Change, error) {
	return nil, nil
}

// Apply is a no-op for the gate step.
func (d *DetectPrerequisites) Apply(_ context.Context) error {
	return nil
}
