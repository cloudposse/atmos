// Package awssso implements the AWS SSO migration steps.
package awssso

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// allowedPermissionSetsMarker is the byte sequence used to detect existing config.
var allowedPermissionSetsMarker = []byte("allowed_permission_sets")

// UpdateTfstateBackend adds allowed_permission_sets to tfstate-backend.yaml.
type UpdateTfstateBackend struct {
	migCtx *migrate.MigrationContext
	fs     migrate.FileSystem
}

// NewUpdateTfstateBackend creates a new update tfstate backend step.
func NewUpdateTfstateBackend(migCtx *migrate.MigrationContext, fs migrate.FileSystem) *UpdateTfstateBackend {
	return &UpdateTfstateBackend{migCtx: migCtx, fs: fs}
}

// Name returns the step identifier.
func (s *UpdateTfstateBackend) Name() string { return "update-tfstate-backend" }

// Description returns a human-readable description of the step.
func (s *UpdateTfstateBackend) Description() string {
	return "Add allowed_permission_sets to tfstate-backend.yaml for SSO access"
}

// Detect checks whether tfstate-backend.yaml exists and already has allowed_permission_sets.
func (s *UpdateTfstateBackend) Detect(ctx context.Context) (migrate.StepStatus, error) {
	defer perf.Track(nil, "awssso.UpdateTfstateBackend.Detect")()

	filePath := s.findTfstateBackendFile()
	if filePath == "" {
		log.Debug("No tfstate-backend.yaml found — step not applicable")
		return migrate.StepNotApplicable, nil
	}

	log.Debug("Found tfstate-backend.yaml", "path", filePath)

	data, err := s.fs.ReadFile(filePath)
	if err != nil {
		return migrate.StepComplete, fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	if bytes.Contains(data, allowedPermissionSetsMarker) {
		log.Debug("allowed_permission_sets already configured")
		return migrate.StepComplete, nil
	}

	log.Debug("allowed_permission_sets not found — update needed")
	return migrate.StepNeeded, nil
}

// Plan returns a Change describing the allowed_permission_sets block to add.
func (s *UpdateTfstateBackend) Plan(ctx context.Context) ([]migrate.Change, error) {
	defer perf.Track(nil, "awssso.UpdateTfstateBackend.Plan")()

	filePath := s.findTfstateBackendFile()
	if filePath == "" {
		return nil, nil
	}

	block := s.generatePermissionSetsBlock()

	return []migrate.Change{
		{
			FilePath:    filePath,
			Description: fmt.Sprintf("Add allowed_permission_sets for accounts: %s", strings.Join(s.sortedAccountNames(), ", ")),
			Detail:      block,
		},
	}, nil
}

// Apply reads the tfstate-backend.yaml file, appends the allowed_permission_sets block, and writes it back.
func (s *UpdateTfstateBackend) Apply(ctx context.Context) error {
	defer perf.Track(nil, "awssso.UpdateTfstateBackend.Apply")()

	filePath := s.findTfstateBackendFile()
	if filePath == "" {
		return nil
	}

	existing, err := s.fs.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	if bytes.Contains(existing, allowedPermissionSetsMarker) {
		return nil
	}

	block := s.generatePermissionSetsBlock()
	content := append(existing, []byte("\n"+block)...)

	if err := s.fs.WriteFile(filePath, content, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	return nil
}

// findTfstateBackendFile returns the path to tfstate-backend.yaml if it exists.
func (s *UpdateTfstateBackend) findTfstateBackendFile() string {
	base := s.migCtx.StacksBasePath

	candidates := []string{
		filepath.Join(base, "catalog", "tfstate-backend.yaml"),
		filepath.Join(base, "catalog", "tfstate-backend", "tfstate-backend.yaml"),
	}

	for _, path := range candidates {
		if s.fs.Exists(path) {
			return path
		}
	}

	return ""
}

// sortedAccountNames returns the account names from the migration context sorted alphabetically.
func (s *UpdateTfstateBackend) sortedAccountNames() []string {
	names := make([]string, 0, len(s.migCtx.AccountMap))
	for name := range s.migCtx.AccountMap {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// generatePermissionSetsBlock returns the YAML block for allowed_permission_sets.
func (s *UpdateTfstateBackend) generatePermissionSetsBlock() string {
	accounts := s.sortedAccountNames()

	var b strings.Builder

	b.WriteString("allowed_permission_sets:\n")

	for _, name := range accounts {
		b.WriteString(fmt.Sprintf("  %s:\n", name))
		b.WriteString("    - AdministratorAccess\n")
		b.WriteString("    - Terraform*Access\n")
	}

	return b.String()
}
