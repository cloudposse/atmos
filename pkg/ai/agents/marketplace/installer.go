package marketplace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/ai/agents"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Installer manages agent installation.
type Installer struct {
	downloader    *Downloader
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

// NewInstaller creates a new agent installer.
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

// Install installs an agent from a source URL.
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
			return fmt.Errorf("%w: use --force to reinstall", ErrAgentAlreadyInstalled)
		}
	}

	// 3. Download to temporary directory.
	fmt.Printf("Downloading agent from %s...\n", sourceInfo.URL)
	tempDir, err := i.downloader.Download(ctx, sourceInfo)
	if err != nil {
		return fmt.Errorf("failed to download agent: %w", err)
	}
	defer os.RemoveAll(tempDir) // Cleanup on error.

	// 4. Parse and validate metadata.
	metadataPath := filepath.Join(tempDir, ".agent.yaml")
	metadata, err := ParseAgentMetadata(metadataPath)
	if err != nil {
		return fmt.Errorf("invalid agent metadata: %w", err)
	}

	// 5. Validate agent.
	if err := i.validator.Validate(tempDir, metadata); err != nil {
		return fmt.Errorf("agent validation failed: %w", err)
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

	// 8. Install agent (move to final location).
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	if err := os.Rename(tempDir, installPath); err != nil {
		return fmt.Errorf("failed to install agent: %w", err)
	}

	// 9. Register agent.
	installedAgent := &InstalledAgent{
		Name:        metadata.Name,
		DisplayName: metadata.DisplayName,
		Source:      sourceInfo.FullPath,
		Version:     metadata.Version,
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

	if err := i.localRegistry.Add(installedAgent); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// 10. Success.
	fmt.Printf("\n✓ Agent %q installed successfully\n", metadata.DisplayName)
	fmt.Printf("  Version: %s\n", metadata.Version)
	fmt.Printf("  Location: %s\n", installPath)
	fmt.Printf("\nUsage: Switch to this agent in the TUI with Ctrl+A\n")

	return nil
}

// Uninstall removes an installed agent.
func (i *Installer) Uninstall(name string, force bool) error {
	defer perf.Track(nil, "marketplace.Installer.Uninstall")()

	// 1. Get agent from registry.
	agent, err := i.localRegistry.Get(name)
	if err != nil {
		return err
	}

	// 2. Confirm uninstallation (unless force).
	if !force {
		fmt.Printf("Uninstall agent %q (version %s)? [y/N] ", agent.DisplayName, agent.Version)
		var response string
		_, _ = fmt.Scanln(&response)

		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			return fmt.Errorf("uninstallation cancelled")
		}
	}

	// 3. Remove agent directory.
	if err := os.RemoveAll(agent.Path); err != nil {
		return fmt.Errorf("failed to remove agent directory: %w", err)
	}

	// 4. Remove from registry.
	if err := i.localRegistry.Remove(name); err != nil {
		return err
	}

	fmt.Printf("✓ Agent %q uninstalled successfully\n", agent.DisplayName)
	return nil
}

// List returns all installed agents.
func (i *Installer) List() []*InstalledAgent {
	defer perf.Track(nil, "marketplace.Installer.List")()
	return i.localRegistry.List()
}

// Get retrieves an installed agent.
func (i *Installer) Get(name string) (*InstalledAgent, error) {
	defer perf.Track(nil, "marketplace.Installer.Get")()
	return i.localRegistry.Get(name)
}

// LoadInstalledAgents loads all installed community agents into the agent registry.
func (i *Installer) LoadInstalledAgents(registry *agents.Registry) error {
	defer perf.Track(nil, "marketplace.Installer.LoadInstalledAgents")()

	for _, installed := range i.localRegistry.List() {
		if !installed.Enabled {
			continue // Skip disabled agents.
		}

		// Parse metadata.
		metadataPath := filepath.Join(installed.Path, ".agent.yaml")
		metadata, err := ParseAgentMetadata(metadataPath)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to load community agent %q: %v", installed.Name, err))
			continue
		}

		// Read prompt.
		promptPath := filepath.Join(installed.Path, metadata.Prompt.File)
		promptContent, err := os.ReadFile(promptPath)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to read prompt for agent %q: %v", installed.Name, err))
			continue
		}

		// Create agent.
		agent := &agents.Agent{
			Name:         metadata.Name,
			DisplayName:  metadata.DisplayName,
			Description:  metadata.Description,
			SystemPrompt: string(promptContent),
			Category:     metadata.Category,
			IsBuiltIn:    false,
		}

		if metadata.Tools != nil {
			agent.AllowedTools = metadata.Tools.Allowed
			agent.RestrictedTools = metadata.Tools.Restricted
		}

		// Register.
		if err := registry.Register(agent); err != nil {
			log.Warn(fmt.Sprintf("Failed to register community agent %q: %v", installed.Name, err))
			continue
		}
	}

	return nil
}

// getInstallPath returns the installation path for an agent.
func (i *Installer) getInstallPath(source *SourceInfo) string {
	agentsDir, _ := GetAgentsDir()
	return filepath.Join(agentsDir, source.FullPath)
}

// confirmInstallation prompts user to confirm agent installation.
func (i *Installer) confirmInstallation(metadata *AgentMetadata) error {
	// Display agent info.
	fmt.Printf("\nAgent: %s\n", metadata.DisplayName)
	fmt.Printf("Author: %s\n", metadata.Author)
	fmt.Printf("Version: %s\n", metadata.Version)
	fmt.Printf("Repository: %s\n", metadata.Repository)

	// Warn about tool access.
	if metadata.Tools != nil && len(metadata.Tools.Allowed) > 0 {
		fmt.Printf("\nTool Access:\n")
		fmt.Printf("  Allowed: %s\n", strings.Join(metadata.Tools.Allowed, ", "))

		// Check for destructive tools.
		destructiveTools := []string{"terraform_apply", "terraform_destroy", "helmfile_apply"}
		hasDestructive := false
		for _, tool := range metadata.Tools.Allowed {
			for _, destructive := range destructiveTools {
				if tool == destructive {
					hasDestructive = true
					break
				}
			}
		}

		if hasDestructive {
			fmt.Printf("\n⚠️  WARNING: This agent requests access to destructive operations.\n")
			fmt.Printf("   Review the agent source before using:\n")
			fmt.Printf("   %s\n", metadata.Repository)
		}
	}

	// Prompt for confirmation.
	fmt.Printf("\nDo you want to install this agent? [y/N] ")
	var response string
	_, _ = fmt.Scanln(&response)

	if strings.ToLower(strings.TrimSpace(response)) != "y" {
		return fmt.Errorf("installation cancelled by user")
	}

	return nil
}
