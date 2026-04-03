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
)

// authIdentityMarkers are byte sequences used to detect existing auth identity config.
var authIdentityMarkers = [][]byte{
	[]byte("terraform:"),
	[]byte("identities:"),
}

// UpdateStackDefaults adds terraform auth identity blocks to _defaults.yaml files.
type UpdateStackDefaults struct {
	migCtx *migrate.MigrationContext
	fs     migrate.FileSystem
}

// NewUpdateStackDefaults creates a new update stack defaults step.
func NewUpdateStackDefaults(migCtx *migrate.MigrationContext, fs migrate.FileSystem) *UpdateStackDefaults {
	return &UpdateStackDefaults{migCtx: migCtx, fs: fs}
}

// Name returns the step identifier.
func (s *UpdateStackDefaults) Name() string { return "update-stack-defaults" }

// Description returns a human-readable description of the step.
func (s *UpdateStackDefaults) Description() string {
	return "Add terraform auth identity defaults to stack _defaults.yaml files"
}

// Detect checks whether all _defaults.yaml files already have auth identity config.
func (s *UpdateStackDefaults) Detect(ctx context.Context) (migrate.StepStatus, error) {
	defer perf.Track(nil, "awssso.UpdateStackDefaults.Detect")()

	files, err := s.findDefaultsFiles()
	if err != nil {
		log.Debug("Error finding _defaults.yaml files", "error", err)
		return migrate.StepComplete, err
	}

	log.Debug("Found _defaults.yaml files", "count", len(files), "files", files)

	if len(files) == 0 {
		log.Debug("No _defaults.yaml files found — step already complete")
		return migrate.StepComplete, nil
	}

	for _, f := range files {
		hasAuth, err := s.fileHasAuthIdentity(f)
		if err != nil {
			return migrate.StepComplete, err
		}

		if !hasAuth {
			log.Debug("File missing auth identity config", "file", f)
			return migrate.StepNeeded, nil
		}
	}

	return migrate.StepComplete, nil
}

// Plan returns one Change per _defaults.yaml file missing the auth identity block.
func (s *UpdateStackDefaults) Plan(ctx context.Context) ([]migrate.Change, error) {
	defer perf.Track(nil, "awssso.UpdateStackDefaults.Plan")()

	files, err := s.findDefaultsFiles()
	if err != nil {
		return nil, err
	}

	var changes []migrate.Change

	for _, f := range files {
		hasAuth, err := s.fileHasAuthIdentity(f)
		if err != nil {
			return nil, err
		}

		if hasAuth {
			continue
		}

		accountName := filepath.Base(filepath.Dir(f))
		block := s.generateIdentityBlock(accountName)

		changes = append(changes, migrate.Change{
			FilePath:    f,
			Description: fmt.Sprintf("Add terraform auth identity for %s", accountName),
			Detail:      block,
		})
	}

	return changes, nil
}

// Apply appends the auth identity block to each _defaults.yaml file that needs it.
func (s *UpdateStackDefaults) Apply(ctx context.Context) error {
	defer perf.Track(nil, "awssso.UpdateStackDefaults.Apply")()

	files, err := s.findDefaultsFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		hasAuth, err := s.fileHasAuthIdentity(f)
		if err != nil {
			return err
		}

		if hasAuth {
			continue
		}

		accountName := filepath.Base(filepath.Dir(f))
		block := s.generateIdentityBlock(accountName)

		existing, err := s.fs.ReadFile(f)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", f, err)
		}

		content := append(existing, []byte("\n"+block)...)

		if err := s.fs.WriteFile(f, content, 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", f, err)
		}
	}

	return nil
}

// findDefaultsFiles returns all _defaults.yaml files under the stacks base path.
func (s *UpdateStackDefaults) findDefaultsFiles() ([]string, error) {
	base := s.migCtx.StacksBasePath

	// Search at two depth levels since filepath.Glob does not support ** recursion.
	patterns := []string{
		filepath.Join(base, "orgs", "*", "*", "*", "_defaults.yaml"),
		filepath.Join(base, "orgs", "*", "*", "_defaults.yaml"),
	}

	var allFiles []string

	for _, pattern := range patterns {
		matches, err := s.fs.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to glob %s: %w", pattern, err)
		}

		allFiles = append(allFiles, matches...)
	}

	return allFiles, nil
}

// fileHasAuthIdentity checks if the file content contains terraform auth identity markers.
func (s *UpdateStackDefaults) fileHasAuthIdentity(path string) (bool, error) {
	data, err := s.fs.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}

	for _, marker := range authIdentityMarkers {
		if !bytes.Contains(data, marker) {
			return false, nil
		}
	}

	return true, nil
}

// generateIdentityBlock returns the YAML block for a terraform auth identity.
func (s *UpdateStackDefaults) generateIdentityBlock(accountName string) string {
	return fmt.Sprintf(`terraform:
  auth:
    identities:
      %s/terraform:
        default: true
`, accountName)
}
