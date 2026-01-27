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
func (i *Installer) Install(ctx context.Context, source string, opts InstallOptions) error {
	defer perf.Track(nil, "marketplace.Installer.Install")()

	// 1. Parse source.
	sourceInfo, err := ParseSource(source)
	if err != nil {
		return fmt.Errorf("invalid source: %w", err)
	}

	// 2. Check if already installed.
	if !opts.Force {
		if _, err := i.localRegistry.Get(sourceInfo.Name); err == nil {
			return fmt.Errorf("%w: use --force to reinstall", ErrSkillAlreadyInstalled)
		}
	}

	// 3. Download to temporary directory.
	fmt.Printf("Downloading skill from %s...\n", sourceInfo.URL)
	tempDir, err := i.downloader.Download(ctx, sourceInfo)
	if err != nil {
		return fmt.Errorf("failed to download skill: %w", err)
	}
	defer os.RemoveAll(tempDir) // Cleanup on error.

	// 4. Parse and validate metadata from SKILL.md.
	skillMDPath := filepath.Join(tempDir, "SKILL.md")
	metadata, err := ParseSkillMetadata(skillMDPath)
	if err != nil {
		return fmt.Errorf("invalid skill metadata: %w", err)
	}

	// 5. Validate skill.
	if err := i.validator.Validate(tempDir, metadata); err != nil {
		return fmt.Errorf("skill validation failed: %w", err)
	}

	// 6. Security check (interactive prompt).
	if !opts.SkipConfirm {
		if err := i.confirmInstallation(metadata); err != nil {
			return err // User cancelled.
		}
	}

	// 7. Determine installation path.
	installPath := i.getInstallPath(sourceInfo)

	// If force reinstall, remove existing installation.
	if opts.Force {
		if err := os.RemoveAll(installPath); err != nil {
			log.Warn(fmt.Sprintf("Failed to remove existing installation: %v", err))
		}
	}

	// 8. Install skill (move to final location).
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := os.Rename(tempDir, installPath); err != nil {
		return fmt.Errorf("failed to install skill: %w", err)
	}

	// 9. Register skill.
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

	// 10. Success.
	fmt.Printf("\n✓ Skill %q installed successfully\n", metadata.GetDisplayName())
	fmt.Printf("  Version: %s\n", metadata.GetVersion())
	fmt.Printf("  Location: %s\n", redactHomePath(installPath))
	fmt.Printf("\nUsage: Switch to this skill in the TUI with Ctrl+A\n")

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
			log.Warn(fmt.Sprintf("Failed to load community skill %q: %v", installed.Name, err))
			continue
		}

		// Read prompt content (body after frontmatter).
		promptContent, err := readSkillPrompt(skillMDPath)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to read prompt for skill %q: %v", installed.Name, err))
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
			log.Warn(fmt.Sprintf("Failed to register community skill %q: %v", installed.Name, err))
			continue
		}
	}

	return nil
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
