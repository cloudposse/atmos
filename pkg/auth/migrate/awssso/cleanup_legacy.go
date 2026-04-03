// Package awssso implements the AWS SSO migration steps.
package awssso

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// legacyAssumeRolePattern is the byte pattern used to detect legacy identity configurations.
var legacyAssumeRolePattern = []byte("kind: aws/assume-role")

// CleanupLegacyAuth detects and reports legacy auth configuration that has been
// superseded by the SSO migration. In v1 this step is advisory-only: it reports
// what the user should clean up manually rather than performing automated removal.
type CleanupLegacyAuth struct {
	migCtx *migrate.MigrationContext
	fs     migrate.FileSystem
}

// NewCleanupLegacyAuth creates a new cleanup legacy auth step.
func NewCleanupLegacyAuth(migCtx *migrate.MigrationContext, fs migrate.FileSystem) *CleanupLegacyAuth {
	return &CleanupLegacyAuth{migCtx: migCtx, fs: fs}
}

// Name returns the step identifier.
func (s *CleanupLegacyAuth) Name() string { return "cleanup-legacy-auth" }

// Description returns a human-readable description of the step.
func (s *CleanupLegacyAuth) Description() string {
	return "Detect and report legacy auth configuration for manual cleanup"
}

// Detect checks for legacy auth patterns that should be removed after SSO migration.
func (s *CleanupLegacyAuth) Detect(ctx context.Context) (migrate.StepStatus, error) {
	defer perf.Track(nil, "awssso.CleanupLegacyAuth.Detect")()

	authDir := s.legacyAuthDirPath()
	log.Debug("Checking for legacy auth directory", "path", authDir)
	if s.hasLegacyAuthDir() {
		log.Debug("Legacy auth directory found", "path", authDir)
		return migrate.StepNeeded, nil
	}

	log.Debug("Checking for legacy assume-role identities", "config", s.migCtx.AtmosConfigPath)
	hasAssumeRole, err := s.hasLegacyAssumeRole()
	if err != nil {
		return migrate.StepNeeded, fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	if hasAssumeRole {
		log.Debug("Legacy aws/assume-role identities found in atmos.yaml")
		return migrate.StepNeeded, nil
	}

	log.Debug("No legacy auth configuration found")
	return migrate.StepComplete, nil
}

// Plan returns the list of changes describing what legacy configuration was found.
func (s *CleanupLegacyAuth) Plan(ctx context.Context) ([]migrate.Change, error) {
	defer perf.Track(nil, "awssso.CleanupLegacyAuth.Plan")()

	var changes []migrate.Change

	if s.hasLegacyAuthDir() {
		authDir := s.legacyAuthDirPath()
		changes = append(changes, migrate.Change{
			FilePath:    authDir,
			Description: "Remove legacy .atmos.d/auth/ directory",
			Detail:      fmt.Sprintf("Directory %s contains legacy auth configuration that is no longer needed.", authDir),
		})
	}

	hasAssumeRole, err := s.hasLegacyAssumeRole()
	if err != nil {
		return nil, fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	if hasAssumeRole {
		changes = append(changes, migrate.Change{
			FilePath:    s.migCtx.AtmosConfigPath,
			Description: "Remove legacy aws/assume-role identities from atmos.yaml",
			Detail:      "File contains identities with kind: aws/assume-role that should be replaced by SSO profiles.",
		})
	}

	return changes, nil
}

// Apply prints advisory warnings about legacy configuration that the user should
// clean up manually. Automated cleanup is deferred to a future version.
func (s *CleanupLegacyAuth) Apply(ctx context.Context) error {
	defer perf.Track(nil, "awssso.CleanupLegacyAuth.Apply")()

	if s.hasLegacyAuthDir() {
		authDir := s.legacyAuthDirPath()
		ui.Warning(fmt.Sprintf("Legacy auth directory found: %s", authDir))
		ui.Writeln("  Please remove this directory manually after verifying the SSO migration is complete.")
	}

	hasAssumeRole, err := s.hasLegacyAssumeRole()
	if err != nil {
		return fmt.Errorf("failed to read atmos.yaml: %w", err)
	}

	if hasAssumeRole {
		ui.Warning(fmt.Sprintf("Legacy aws/assume-role identities found in: %s", s.migCtx.AtmosConfigPath))
		ui.Writeln("  Please remove these identities manually and use SSO profiles instead.")
	}

	return nil
}

// hasLegacyAuthDir returns true if the .atmos.d/auth/ directory exists.
func (s *CleanupLegacyAuth) hasLegacyAuthDir() bool {
	return s.fs.Exists(s.legacyAuthDirPath())
}

// legacyAuthDirPath returns the path to the legacy .atmos.d/auth/ directory.
func (s *CleanupLegacyAuth) legacyAuthDirPath() string {
	projectRoot := filepath.Dir(s.migCtx.AtmosConfigPath)
	return filepath.Join(projectRoot, ".atmos.d", "auth")
}

// hasLegacyAssumeRole reads atmos.yaml and checks for aws/assume-role identity patterns.
func (s *CleanupLegacyAuth) hasLegacyAssumeRole() (bool, error) {
	data, err := s.fs.ReadFile(s.migCtx.AtmosConfigPath)
	if err != nil {
		return false, err
	}

	return bytes.Contains(data, legacyAssumeRolePattern), nil
}
