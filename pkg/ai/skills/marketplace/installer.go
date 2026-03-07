package marketplace

import (
	"bufio"
	"context"
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
	skillMDPath := filepath.Join(tempDir, "SKILL.md")
	if _, err := os.Stat(skillMDPath); err == nil {
		// Single-skill repo: SKILL.md at root.
		return i.installSingleSkill(tempDir, sourceInfo, opts)
	}

	// Check for multi-skill package: skills/*/SKILL.md pattern.
	pattern := filepath.Join(tempDir, "agent-skills", "skills", "*", "SKILL.md")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		// Also try skills/*/SKILL.md as alternative layout.
		pattern = filepath.Join(tempDir, "skills", "*", "SKILL.md")
		matches, err = filepath.Glob(pattern)
	}
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("invalid skill metadata: no SKILL.md found at root or in skills/*/SKILL.md pattern")
	}

	return i.installMultiSkillPackage(matches, sourceInfo, opts)
}

// installSingleSkill installs a single skill from a downloaded repository.
func (i *Installer) installSingleSkill(tempDir string, sourceInfo *SourceInfo, opts InstallOptions) error {
	// Parse and validate metadata from SKILL.md.
	skillMDPath := filepath.Join(tempDir, "SKILL.md")
	metadata, err := ParseSkillMetadata(skillMDPath)
	if err != nil {
		return fmt.Errorf("invalid skill metadata: %w", err)
	}

	// Validate skill.
	if err := i.validator.Validate(tempDir, metadata); err != nil {
		return fmt.Errorf("skill validation failed: %w", err)
	}

	// Security check (interactive prompt).
	if !opts.SkipConfirm {
		if err := i.confirmInstallation(metadata); err != nil {
			return err // User cancelled.
		}
	}

	// Determine installation path.
	installPath := i.getInstallPath(sourceInfo)

	// If force reinstall, remove existing installation.
	if opts.Force {
		if err := os.RemoveAll(installPath); err != nil {
			log.Warnf("Failed to remove existing installation: %v", err)
		}
	}

	// Install skill (move to final location).
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := os.Rename(tempDir, installPath); err != nil {
		return fmt.Errorf("failed to install skill: %w", err)
	}

	// Register skill.
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
	}

	// Remove old registration if force reinstalling.
	if opts.Force {
		_ = i.localRegistry.Remove(metadata.Name) // Ignore error if not exists.
	}

	if err := i.localRegistry.Add(installedSkill); err != nil {
		return fmt.Errorf("failed to register skill: %w", err)
	}

	// Success.
	fmt.Printf("\n✓ Skill %q installed successfully\n", metadata.GetDisplayName())
	fmt.Printf("  Version: %s\n", metadata.GetVersion())
	fmt.Printf("  Location: %s\n", redactHomePath(installPath))
	fmt.Printf("\nUsage: Switch to this skill in the TUI with Ctrl+A\n")

	return nil
}

// installMultiSkillPackage installs multiple skills discovered in a package.
func (i *Installer) installMultiSkillPackage(skillMDPaths []string, sourceInfo *SourceInfo, opts InstallOptions) error {
	// Discover all skills and their metadata.
	type discoveredSkill struct {
		dir      string
		metadata *SkillMetadata
	}

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

	if len(discovered) == 0 {
		return fmt.Errorf("no valid skills found in package")
	}

	// Show discovery summary.
	fmt.Printf("Discovered %d skills in package:\n", len(discovered))
	fmt.Printf("  %s\n", strings.Join(skillNames, ", "))

	// Confirmation prompt for multi-skill install.
	if !opts.SkipConfirm {
		fmt.Printf("\nInstall all %d skills? [y/N] ", len(discovered))
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			return fmt.Errorf("installation cancelled by user")
		}
	}

	fmt.Println()

	// Install each skill.
	installed := 0
	for _, skill := range discovered {
		skillName := skill.metadata.Name

		// Determine installation path per skill.
		skillsDir, _ := GetSkillsDir()
		installPath := filepath.Join(skillsDir, sourceInfo.Owner, sourceInfo.Repo, skillName)

		// If force reinstall, remove existing.
		if opts.Force {
			if err := os.RemoveAll(installPath); err != nil {
				log.Warnf("Failed to remove existing installation for %s: %v", skillName, err)
			}
			_ = i.localRegistry.Remove(skillName)
		} else {
			// Skip if already installed.
			if _, err := i.localRegistry.Get(skillName); err == nil {
				fmt.Printf("  Skipping %s (already installed, use --force to reinstall)\n", skillName)
				continue
			}
		}

		// Copy skill directory to install path.
		if err := os.MkdirAll(installPath, 0o755); err != nil {
			log.Warnf("Failed to create directory for %s: %v", skillName, err)
			continue
		}

		if err := copyDir(skill.dir, installPath); err != nil {
			log.Warnf("Failed to install skill %s: %v", skillName, err)
			continue
		}

		// Register skill.
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
		}

		if err := i.localRegistry.Add(installedSkill); err != nil {
			log.Warnf("Failed to register skill %s: %v", skillName, err)
			continue
		}

		fmt.Printf("  Installing %s... done\n", skillName)
		installed++
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

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o600); err != nil {
				return err
			}
		}
	}

	return nil
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
			return fmt.Errorf("uninstallation cancelled")
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

		// Parse metadata from SKILL.md.
		skillMDPath := filepath.Join(installed.Path, "SKILL.md")
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
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
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

// confirmInstallation prompts user to confirm skill installation.
func (i *Installer) confirmInstallation(metadata *SkillMetadata) error {
	// Display skill info.
	fmt.Printf("\nSkill: %s\n", metadata.GetDisplayName())
	fmt.Printf("Author: %s\n", metadata.GetAuthor())
	fmt.Printf("Version: %s\n", metadata.GetVersion())
	fmt.Printf("Repository: %s\n", metadata.GetRepository())

	// Warn about tool access.
	if len(metadata.AllowedTools) > 0 {
		fmt.Printf("\nTool Access:\n")
		fmt.Printf("  Allowed: %s\n", strings.Join(metadata.AllowedTools, ", "))

		// Check for destructive tools.
		destructiveTools := []string{"terraform_apply", "terraform_destroy", "helmfile_apply"}
		hasDestructive := false
		for _, tool := range metadata.AllowedTools {
			for _, destructive := range destructiveTools {
				if tool == destructive {
					hasDestructive = true
					break
				}
			}
		}

		if hasDestructive {
			fmt.Printf("\n⚠️  WARNING: This skill requests access to destructive operations.\n")
			fmt.Printf("   Review the skill source before using:\n")
			fmt.Printf("   %s\n", metadata.GetRepository())
		}
	}

	// Prompt for confirmation.
	fmt.Printf("\nDo you want to install this skill? [y/N] ")
	var response string
	_, _ = fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return fmt.Errorf("installation cancelled by user")
	}

	return nil
}
