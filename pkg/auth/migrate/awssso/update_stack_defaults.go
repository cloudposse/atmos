// Package awssso implements the AWS SSO migration steps.
package awssso

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// authIdentityMarkers are byte sequences that must ALL be present to indicate
// the file already has auth identity config.
var authIdentityMarkers = [][]byte{
	[]byte("auth:"),
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

		changes = append(changes, migrate.Change{
			FilePath:    f,
			Description: fmt.Sprintf("Add terraform auth identity for %s", accountName),
			Detail:      authIdentitySubBlock(accountName),
		})
	}

	return changes, nil
}

// Apply adds the auth identity block to each _defaults.yaml file that needs it.
// If the file already has a top-level `terraform:` key, the auth block is merged
// into it to avoid duplicate keys.
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

		existing, err := s.fs.ReadFile(f)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", f, err)
		}

		accountName := filepath.Base(filepath.Dir(f))
		content := mergeAuthIdentityBlock(existing, accountName)

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

// authIdentitySubBlock returns the auth identity lines to insert under an existing
// `terraform:` key (indented at 2 spaces).
func authIdentitySubBlock(accountName string) string {
	return fmt.Sprintf("  auth:\n    identities:\n      %s/terraform:\n        default: true\n", accountName)
}

// mergeAuthIdentityBlock inserts the auth identity block into the file content.
// If a top-level `terraform:` key already exists, the auth sub-block is inserted
// at the end of that section. Otherwise, a new `terraform:` block is appended.
func mergeAuthIdentityBlock(existing []byte, accountName string) []byte {
	lines := splitLines(existing)
	terraformIdx := -1

	// Find the top-level `terraform:` line.
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "terraform:" {
			terraformIdx = i
			break
		}
	}

	if terraformIdx == -1 {
		// No existing terraform key — append a full block.
		block := fmt.Sprintf("\nterraform:\n%s", authIdentitySubBlock(accountName))
		return append(existing, []byte(block)...)
	}

	// Find the end of the terraform section: the next top-level key (non-blank,
	// non-comment line at column 0) or end of file.
	insertIdx := len(lines)
	for i := terraformIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" || strings.TrimSpace(line) == "" {
			continue
		}
		// A non-blank line starting at column 0 (not indented) is the next top-level key.
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
			insertIdx = i
			break
		}
	}

	// Build the result: lines before insert point, auth block, remaining lines.
	var buf bytes.Buffer
	for i := 0; i < insertIdx; i++ {
		buf.WriteString(lines[i])
		buf.WriteByte('\n')
	}
	buf.WriteString(authIdentitySubBlock(accountName))
	for i := insertIdx; i < len(lines); i++ {
		buf.WriteString(lines[i])
		if i < len(lines)-1 {
			buf.WriteByte('\n')
		}
	}

	// Preserve trailing newline if the original had one.
	if len(existing) > 0 && existing[len(existing)-1] == '\n' && buf.Len() > 0 {
		result := buf.Bytes()
		if result[len(result)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}

	return buf.Bytes()
}

// splitLines splits content into lines without the trailing newline character.
func splitLines(data []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
