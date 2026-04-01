package marketplace

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/ai/skills"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Installer manages skill installation.
type Installer struct {
	downloader    DownloaderInterface
	validator     *Validator
	localRegistry *LocalRegistry
	atmosVersion  string
}

// InstallOptions configures the installation process.
type InstallOptions struct {
	Force       bool   // Reinstall if already installed.
	SkipConfirm bool   // Skip security confirmation prompt.
	CustomName  string // Install with custom name (--as).
}

// NewInstaller creates a new skill installer.
func NewInstaller(atmosVersion string) (*Installer, error) {
	defer perf.Track(nil, "marketplace.NewInstaller")()

	localRegistry, err := NewLocalRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to load local registry: %w", err)
	}

	return &Installer{
		downloader:    NewDownloader(),
		validator:     NewValidator(atmosVersion),
		localRegistry: localRegistry,
		atmosVersion:  atmosVersion,
	}, nil
}

// Install installs a skill from a source URL.
// It supports both single-skill repos (with SKILL.md at root) and multi-skill packages
// (with skills/*/SKILL.md pattern).
func (i *Installer) Install(ctx context.Context, source string, opts InstallOptions) error {
	defer perf.Track(nil, "marketplace.Installer.Install")()

	// 1. Parse source.
	sourceInfo, err := ParseSource(source)
	if err != nil {
		return fmt.Errorf("invalid source: %w", err)
	}

	// 2. Check if already installed (for single-skill only; multi-skill checked per skill).
	if !opts.Force {
		if _, err := i.localRegistry.Get(sourceInfo.Name); err == nil {
			return fmt.Errorf("%w: use --force to reinstall", ErrSkillAlreadyInstalled)
		}
	}

	// 3. Download to temporary directory.
	fmt.Printf("Downloading skills from %s...\n", sourceInfo.URL)
	tempDir, err := i.downloader.Download(ctx, sourceInfo)
	if err != nil {
		return fmt.Errorf("failed to download skill: %w", err)
	}
	defer os.RemoveAll(tempDir) // Cleanup on error.

	// 4. Check if this is a single-skill repo or a multi-skill package.
	skillMDPath := filepath.Join(tempDir, skillFileName)
	if _, err := os.Stat(skillMDPath); err == nil {
		// Single-skill repo: SKILL.md at root.
		return i.installSingleSkill(tempDir, sourceInfo, opts)
	}

	// Check for multi-skill package: skills/*/SKILL.md pattern.
	pattern := filepath.Join(tempDir, "agent-skills", "skills", "*", skillFileName)
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		// Also try skills/*/SKILL.md as alternative layout.
		pattern = filepath.Join(tempDir, "skills", "*", skillFileName)
		matches, err = filepath.Glob(pattern)
	}
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("%w: no %s found at root or in skills/*/%s pattern", ErrInvalidMetadata, skillFileName, skillFileName)
	}

	return i.installMultiSkillPackage(matches, sourceInfo, opts)
}

// parseAndValidateSkill parses metadata from SKILL.md and validates the skill.
func (i *Installer) parseAndValidateSkill(tempDir string) (*SkillMetadata, error) {
	skillMDPath := filepath.Join(tempDir, skillFileName)
	metadata, err := ParseSkillMetadata(skillMDPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidMetadata, err)
	}

	if err := i.validator.Validate(tempDir, metadata); err != nil {
		return nil, fmt.Errorf("skill validation failed: %w", err)
	}

	return metadata, nil
}

// moveSkillToInstallPath moves the skill from temp dir to its final install path.
func (i *Installer) moveSkillToInstallPath(tempDir, installPath string, force bool) error {
	if force {
		if err := os.RemoveAll(installPath); err != nil {
			log.Warnf("Failed to remove existing installation: %v", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(installPath), dirPermissions); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := os.Rename(tempDir, installPath); err != nil {
		return fmt.Errorf("failed to install skill: %w", err)
	}

	return nil
}

// registerSkill registers a skill in the local registry.
func (i *Installer) registerSkill(metadata *SkillMetadata, sourceInfo *SourceInfo, installPath, contentHash string, force bool) error {
	installedSkill := &InstalledSkill{
		Name:        metadata.Name,
		DisplayName: metadata.GetDisplayName(),
		Source:      sourceInfo.FullPath,
		Version:     metadata.GetVersion(),
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        installPath,
		IsBuiltIn:   false,
		Enabled:     true,
		ContentHash: contentHash,
	}

	if force {
		_ = i.localRegistry.Remove(metadata.Name) // Ignore error if not exists.
	}

	if err := i.localRegistry.Add(installedSkill); err != nil {
		return fmt.Errorf("failed to register skill: %w", err)
	}

	return nil
}

// computeSkillHash computes a SHA-256 content hash over all files in skillDir,
// walking subdirectories in deterministic (lexicographic) order and skipping .git.
// Each file contributes its forward-slash relative path and raw content to the hash,
// so the result is identical on Windows and Unix for the same skill content.
func computeSkillHash(skillDir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(skillDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		relPath, relErr := filepath.Rel(skillDir, path)
		if relErr != nil {
			return fmt.Errorf("failed to get relative path: %w", relErr)
		}
		// Normalise to forward slashes so the hash is platform-independent.
		fmt.Fprintf(h, "\x00%s\x00", filepath.ToSlash(relPath))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("failed to read %s: %w", relPath, readErr)
		}
		_, writeErr := h.Write(data)
		return writeErr
	})
	if err != nil {
		return "", fmt.Errorf("failed to compute skill hash: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifySkillHash recomputes the content hash for skillDir and checks it against expected.
// Returns ErrSkillHashMismatch when the hashes differ.
func verifySkillHash(skillDir, expected string) error {
	actual, err := computeSkillHash(skillDir)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%w", ErrSkillHashMismatch)
	}
	return nil
}

// installSingleSkill installs a single skill from a downloaded repository.
func (i *Installer) installSingleSkill(tempDir string, sourceInfo *SourceInfo, opts InstallOptions) error {
	metadata, err := i.parseAndValidateSkill(tempDir)
	if err != nil {
		return err
	}

	// Compute content hash before moving the skill to its final location.
	contentHash, err := computeSkillHash(tempDir)
	if err != nil {
		return fmt.Errorf("failed to compute content hash: %w", err)
	}

	// Security check (interactive prompt).
	if !opts.SkipConfirm {
		if err := i.confirmInstallation(metadata, contentHash); err != nil {
			return err // User cancelled.
		}
	}

	installPath := i.getInstallPath(sourceInfo)

	if err := i.moveSkillToInstallPath(tempDir, installPath, opts.Force); err != nil {
		return err
	}

	if err := i.registerSkill(metadata, sourceInfo, installPath, contentHash, opts.Force); err != nil {
		return err
	}

	// Success.
	fmt.Printf("\n✓ Skill %q installed successfully\n", metadata.GetDisplayName())
	fmt.Printf("  Version: %s\n", metadata.GetVersion())
	fmt.Printf("  Location: %s\n", redactHomePath(installPath))
	fmt.Printf("\nUsage: Switch to this skill in the TUI with Ctrl+A\n")

	return nil
}

// discoveredSkill holds the directory and metadata for a discovered skill.
type discoveredSkill struct {
	dir      string
	metadata *SkillMetadata
}

// discoverSkillsFromPaths parses metadata from each SKILL.md path and returns valid skills.
func discoverSkillsFromPaths(skillMDPaths []string) ([]discoveredSkill, []string) {
	var discovered []discoveredSkill
	var skillNames []string

	for _, skillMDPath := range skillMDPaths {
		skillDir := filepath.Dir(skillMDPath)
		metadata, err := ParseSkillMetadata(skillMDPath)
		if err != nil {
			log.Warnf("Skipping invalid skill at %s: %v", skillMDPath, err)
			continue
		}

		discovered = append(discovered, discoveredSkill{dir: skillDir, metadata: metadata})
		skillNames = append(skillNames, metadata.Name)
	}

	return discovered, skillNames
}

// confirmMultiSkillInstall prompts the user for multi-skill install confirmation.
func confirmMultiSkillInstall(count int) error {
	fmt.Printf("\nInstall all %d skills? [y/N] ", count)
	var response string
	_, _ = fmt.Scanln(&response)
	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return ErrInstallationCancelled
	}
	return nil
}

// installOneSkillFromPackage installs a single skill from a multi-skill package.
// Returns true if the skill was successfully installed.
func (i *Installer) installOneSkillFromPackage(skill discoveredSkill, sourceInfo *SourceInfo, opts InstallOptions) bool {
	skillName := skill.metadata.Name
	skillsDir, _ := GetSkillsDir()
	installPath := filepath.Join(skillsDir, sourceInfo.Owner, sourceInfo.Repo, skillName)

	if opts.Force {
		if err := os.RemoveAll(installPath); err != nil {
			log.Warnf("Failed to remove existing installation for %s: %v", skillName, err)
		}
		_ = i.localRegistry.Remove(skillName)
	} else if _, err := i.localRegistry.Get(skillName); err == nil {
		fmt.Printf("  Skipping %s (already installed, use --force to reinstall)\n", skillName)
		return false
	}

	if err := os.MkdirAll(installPath, dirPermissions); err != nil {
		log.Warnf("Failed to create directory for %s: %v", skillName, err)
		return false
	}

	// Compute content hash from the source directory before copying.
	contentHash, err := computeSkillHash(skill.dir)
	if err != nil {
		log.Warnf("Failed to compute content hash for %s: %v", skillName, err)
		return false
	}

	if err := copyDir(skill.dir, installPath); err != nil {
		log.Warnf("Failed to install skill %s: %v", skillName, err)
		return false
	}

	installedSkill := &InstalledSkill{
		Name:        skillName,
		DisplayName: skill.metadata.GetDisplayName(),
		Source:      sourceInfo.FullPath,
		Version:     skill.metadata.GetVersion(),
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        installPath,
		IsBuiltIn:   false,
		Enabled:     true,
		ContentHash: contentHash,
	}

	if err := i.localRegistry.Add(installedSkill); err != nil {
		log.Warnf("Failed to register skill %s: %v", skillName, err)
		return false
	}

	fmt.Printf("  Installing %s... done\n", skillName)
	return true
}

// installMultiSkillPackage installs multiple skills discovered in a package.
func (i *Installer) installMultiSkillPackage(skillMDPaths []string, sourceInfo *SourceInfo, opts InstallOptions) error {
	discovered, skillNames := discoverSkillsFromPaths(skillMDPaths)
	if len(discovered) == 0 {
		return ErrNoValidSkillsFound
	}

	// Show discovery summary.
	fmt.Printf("Discovered %d skills in package:\n", len(discovered))
	fmt.Printf("  %s\n", strings.Join(skillNames, ", "))

	if !opts.SkipConfirm {
		if err := confirmMultiSkillInstall(len(discovered)); err != nil {
			return err
		}
	}

	fmt.Println()

	installed := 0
	for _, skill := range discovered {
		if i.installOneSkillFromPackage(skill, sourceInfo, opts) {
			installed++
		}
	}

	fmt.Printf("\n%d skills installed successfully.\n", installed)
	fmt.Printf("\nUsage: Switch skills in the TUI with Ctrl+A\n")

	return nil
}

// copyDir copies the contents of a source directory to a destination directory.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if err := copyEntry(srcPath, dstPath, entry.IsDir()); err != nil {
			return err
		}
	}

	return nil
}

// copyEntry copies a single directory entry (file or subdirectory).
func copyEntry(srcPath, dstPath string, isDir bool) error {
	if isDir {
		if err := os.MkdirAll(dstPath, dirPermissions); err != nil {
			return err
		}
		return copyDir(srcPath, dstPath)
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, data, filePermissions)
}

// Uninstall removes an installed skill.
func (i *Installer) Uninstall(name string, force bool) error {
	defer perf.Track(nil, "marketplace.Installer.Uninstall")()

	// 1. Get skill from registry.
	skill, err := i.localRegistry.Get(name)
	if err != nil {
		return err
	}

	// 2. Confirm uninstallation (unless force).
	if !force {
		fmt.Printf("Uninstall skill %q (version %s)? [y/N] ", skill.DisplayName, skill.Version)
		var response string
		_, _ = fmt.Scanln(&response)

		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			return ErrUninstallationCancelled
		}
	}

	// 3. Remove skill directory.
	if err := os.RemoveAll(skill.Path); err != nil {
		return fmt.Errorf("failed to remove skill directory: %w", err)
	}

	// 4. Remove from registry.
	if err := i.localRegistry.Remove(name); err != nil {
		return err
	}

	fmt.Printf("✓ Skill %q uninstalled successfully\n", skill.DisplayName)
	return nil
}

// List returns all installed skills.
func (i *Installer) List() []*InstalledSkill {
	defer perf.Track(nil, "marketplace.Installer.List")()
	return i.localRegistry.List()
}

// Get retrieves an installed skill.
func (i *Installer) Get(name string) (*InstalledSkill, error) {
	defer perf.Track(nil, "marketplace.Installer.Get")()
	return i.localRegistry.Get(name)
}

// LoadInstalledSkills loads all installed community skills into the skill registry.
func (i *Installer) LoadInstalledSkills(registry *skills.Registry) error {
	defer perf.Track(nil, "marketplace.Installer.LoadInstalledSkills")()

	for _, installed := range i.localRegistry.List() {
		if !installed.Enabled {
			continue // Skip disabled skills.
		}

		// Verify content integrity when a hash was recorded at install time.
		// Skills installed before this feature was introduced have an empty hash and are loaded without verification.
		if installed.ContentHash != "" {
			if err := verifySkillHash(installed.Path, installed.ContentHash); err != nil {
				log.Warnf("Integrity check failed for skill %q: %v — skipping", installed.Name, err)
				continue
			}
		}

		// Parse metadata from SKILL.md.
		skillMDPath := filepath.Join(installed.Path, skillFileName)
		metadata, err := ParseSkillMetadata(skillMDPath)
		if err != nil {
			log.Warnf("Failed to load community skill %q: %v", installed.Name, err)
			continue
		}

		// Read prompt content with references.
		promptContent, err := readSkillPromptWithReferences(installed.Path, metadata)
		if err != nil {
			log.Warnf("Failed to read prompt for skill %q: %v", installed.Name, err)
			continue
		}

		// Create skill.
		skill := &skills.Skill{
			Name:            metadata.Name,
			DisplayName:     metadata.GetDisplayName(),
			Description:     metadata.Description,
			SystemPrompt:    promptContent,
			Category:        metadata.GetCategory(),
			IsBuiltIn:       false,
			AllowedTools:    metadata.AllowedTools,
			RestrictedTools: metadata.RestrictedTools,
		}

		// Register.
		if err := registry.Register(skill); err != nil {
			log.Warnf("Failed to register community skill %q: %v", installed.Name, err)
			continue
		}
	}

	return nil
}

// readSkillPromptWithReferences reads the skill prompt body and appends any referenced files.
func readSkillPromptWithReferences(skillDir string, metadata *SkillMetadata) (string, error) {
	skillMDPath := filepath.Join(skillDir, skillFileName)
	body, err := readSkillPrompt(skillMDPath)
	if err != nil {
		return "", err
	}

	// Append reference files if any.
	for _, ref := range metadata.References {
		refPath := filepath.Join(skillDir, ref)
		refContent, err := os.ReadFile(refPath)
		if err != nil {
			log.Warnf("Failed to read reference file %q: %v", ref, err)
			continue
		}

		refName := filepath.Base(ref)
		body += "\n\n---\n\n## Reference: " + refName + "\n\n" + strings.TrimSpace(string(refContent))
	}

	return body, nil
}

// readSkillPrompt reads the markdown body from a SKILL.md file (after frontmatter).
func readSkillPrompt(skillMDPath string) (string, error) {
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		return "", err
	}

	// Extract body after frontmatter.
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	var lines []string
	inFrontmatter := false
	frontmatterEnded := false
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter && lineNum == 1 {
				inFrontmatter = true
				continue
			} else if inFrontmatter {
				inFrontmatter = false
				frontmatterEnded = true
				continue
			}
		}

		if inFrontmatter {
			continue
		}

		if frontmatterEnded {
			lines = append(lines, line)
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n")), scanner.Err()
}

// getInstallPath returns the installation path for a skill.
func (i *Installer) getInstallPath(source *SourceInfo) string {
	skillsDir, _ := GetSkillsDir()
	return filepath.Join(skillsDir, source.FullPath)
}

// redactHomePath replaces the home directory portion of a path with ~ for display.
// This avoids logging sensitive home directory paths while still providing useful information.
func redactHomePath(path string) string {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "~/.atmos/skills/..."
	}
	if strings.HasPrefix(path, homeDir) {
		return "~" + strings.TrimPrefix(path, homeDir)
	}
	return path
}

// hasDestructiveTools checks if any of the allowed tools are destructive.
func hasDestructiveTools(allowedTools []string) bool {
	destructiveTools := []string{"terraform_apply", "terraform_destroy", "helmfile_apply"}
	for _, tool := range allowedTools {
		for _, destructive := range destructiveTools {
			if tool == destructive {
				return true
			}
		}
	}
	return false
}

// printToolAccessWarnings prints tool access information and warnings for destructive tools.
func printToolAccessWarnings(metadata *SkillMetadata) {
	if len(metadata.AllowedTools) == 0 {
		return
	}

	fmt.Printf("\nTool Access:\n")
	fmt.Printf("  Allowed: %s\n", strings.Join(metadata.AllowedTools, ", "))

	if !hasDestructiveTools(metadata.AllowedTools) {
		return
	}

	fmt.Printf("\n⚠️  WARNING: This skill requests access to destructive operations.\n")
	fmt.Printf("   Review the skill source before using:\n")
	fmt.Printf("   %s\n", metadata.GetRepository())
}

// confirmInstallation prompts user to confirm skill installation.
func (i *Installer) confirmInstallation(metadata *SkillMetadata, contentHash string) error {
	// Display skill info.
	fmt.Printf("\nSkill: %s\n", metadata.GetDisplayName())
	fmt.Printf("Author: %s\n", metadata.GetAuthor())
	fmt.Printf("Version: %s\n", metadata.GetVersion())
	fmt.Printf("Repository: %s\n", metadata.GetRepository())
	fmt.Printf("SHA-256: %s\n", contentHash)

	printToolAccessWarnings(metadata)

	// Prompt for confirmation.
	fmt.Printf("\nDo you want to install this skill? [y/N] ")
	var response string
	_, _ = fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return ErrInstallationCancelled
	}

	return nil
}
