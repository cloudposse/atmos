package ai

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/memory"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// aiMemoryCmd represents the ai memory command.
var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage AI project memory",
	Long: `Manage project memory (ATMOS.md) for the AI assistant.

Project memory allows the AI to remember project-specific context, patterns,
and conventions. This includes organization details, common commands, stack
patterns, and infrastructure conventions.

Available operations:
- Initialize memory file with template
- Display current memory content
- Validate memory file format
- Edit memory in your preferred editor
- Show memory file path`,
	RunE: showMemoryCommand,
}

// aiMemoryInitCmd initializes project memory.
var memoryInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize project memory file",
	Long: `Create a new ATMOS.md template file in your project.

This creates a template file with all standard sections:
- Project Context
- Common Commands
- Stack Patterns
- Frequent Issues & Solutions
- Infrastructure Patterns
- Component Catalog Structure
- Team Conventions
- Recent Learnings

Example:
  atmos ai memory init`,
	RunE: initMemoryCommand,
}

// aiMemoryShowCmd displays memory content.
var memoryShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display project memory content",
	Long: `Show the current project memory content.

Displays the formatted memory content that is sent to the AI assistant.
Only includes sections that are enabled in your configuration.

Example:
  atmos ai memory show`,
	RunE: showMemoryCommand,
}

// aiMemoryValidateCmd validates memory format.
var memoryValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate project memory file",
	Long: `Validate the ATMOS.md file format.

Checks that:
- File exists and is readable
- Markdown is properly formatted
- Sections can be parsed correctly
- No syntax errors

Example:
  atmos ai memory validate`,
	RunE: validateMemoryCommand,
}

// aiMemoryEditCmd opens memory in editor.
var memoryEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit project memory in your editor",
	Long: `Open the ATMOS.md file in your preferred text editor.

Uses the editor specified in $EDITOR environment variable,
or falls back to 'vim' if not set.

Example:
  atmos ai memory edit`,
	RunE: editMemoryCommand,
}

// aiMemoryPathCmd shows memory file path.
var memoryPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show project memory file path",
	Long: `Display the absolute path to the ATMOS.md file.

Useful for locating the memory file or integrating with scripts.

Example:
  atmos ai memory path`,
	RunE: pathMemoryCommand,
}

func init() {
	// Add memory command to ai command.
	aiCmd.AddCommand(memoryCmd)

	// Add subcommands to memory command.
	memoryCmd.AddCommand(memoryInitCmd)
	memoryCmd.AddCommand(memoryShowCmd)
	memoryCmd.AddCommand(memoryValidateCmd)
	memoryCmd.AddCommand(memoryEditCmd)
	memoryCmd.AddCommand(memoryPathCmd)

	// Add flags.
	memoryInitCmd.Flags().Bool("force", false, "Overwrite existing ATMOS.md file")
}

// initMemoryManager initializes memory manager with configuration.
func initMemoryManager() (*memory.Manager, *schema.AtmosConfiguration, error) {
	// Initialize configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, nil, err
	}

	// Check if AI is enabled.
	if !isAIEnabled(&atmosConfig) {
		return nil, nil, fmt.Errorf("%w: Set 'settings.ai.enabled: true' in atmos.yaml", errUtils.ErrAINotEnabled)
	}

	// Create memory config (allow disabled memory for some commands).
	memConfig := &memory.Config{
		Enabled:      atmosConfig.Settings.AI.Memory.Enabled,
		FilePath:     atmosConfig.Settings.AI.Memory.FilePath,
		AutoUpdate:   atmosConfig.Settings.AI.Memory.AutoUpdate,
		CreateIfMiss: atmosConfig.Settings.AI.Memory.CreateIfMiss,
		Sections:     atmosConfig.Settings.AI.Memory.Sections,
	}

	// Create memory manager.
	manager := memory.NewManager(atmosConfig.BasePath, memConfig)

	return manager, &atmosConfig, nil
}

// getMemoryFilePath returns the absolute path to ATMOS.md.
func getMemoryFilePath(atmosConfig *schema.AtmosConfiguration) string {
	filePath := atmosConfig.Settings.AI.Memory.FilePath
	if filePath == "" {
		filePath = "ATMOS.md"
	}

	// Make absolute if relative.
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(atmosConfig.BasePath, filePath)
	}

	return filePath
}

// initMemoryCommand initializes project memory.
func initMemoryCommand(cmd *cobra.Command, args []string) error {
	log.Debug("Initializing project memory")

	manager, atmosConfig, err := initMemoryManager()
	if err != nil {
		return err
	}

	filePath := getMemoryFilePath(atmosConfig)

	// Check if file already exists.
	force, _ := cmd.Flags().GetBool("force")
	if !force {
		if _, err := os.Stat(filePath); err == nil {
			return fmt.Errorf("%w: %s (use --force to overwrite)", errUtils.ErrAIProjectMemoryExists, filePath)
		}
	}

	// Create default template.
	ctx := context.Background()
	if err := manager.CreateDefault(ctx); err != nil {
		return fmt.Errorf("failed to create memory file: %w", err)
	}

	u.PrintMessage(fmt.Sprintf("✅ Created ATMOS.md at: %s", filePath))
	u.PrintMessage("\nEdit this file to customize your project memory.")
	u.PrintMessage("Then enable memory in atmos.yaml:")
	u.PrintMessage("  ai:")
	u.PrintMessage("    memory:")
	u.PrintMessage("      enabled: true")

	return nil
}

// showMemoryCommand displays memory content.
func showMemoryCommand(cmd *cobra.Command, args []string) error {
	log.Debug("Showing project memory")

	manager, atmosConfig, err := initMemoryManager()
	if err != nil {
		return err
	}

	filePath := getMemoryFilePath(atmosConfig)

	// Check if file exists.
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		u.PrintMessage("No memory file found.")
		u.PrintMessage("\nCreate one with: atmos ai memory init")
		return nil
	}

	// Check if memory is enabled.
	if !atmosConfig.Settings.AI.Memory.Enabled {
		u.PrintMessage("⚠️  Memory is disabled in configuration.")
		u.PrintMessage("\nEnable it in atmos.yaml:")
		u.PrintMessage("  ai:")
		u.PrintMessage("    memory:")
		u.PrintMessage("      enabled: true")
		u.PrintMessage(fmt.Sprintf("\nMemory file location: %s", filePath))
		return nil
	}

	// Load memory.
	ctx := context.Background()
	_, err = manager.Load(ctx)
	if err != nil {
		return fmt.Errorf("failed to load memory: %w", err)
	}

	// Get formatted context.
	context := manager.GetContext()
	if context == "" {
		u.PrintMessage("Memory is empty or contains no configured sections.")
		return nil
	}

	u.PrintMessage(context)

	return nil
}

// validateMemoryCommand validates memory file.
func validateMemoryCommand(cmd *cobra.Command, args []string) error {
	log.Debug("Validating project memory")

	manager, atmosConfig, err := initMemoryManager()
	if err != nil {
		return err
	}

	filePath := getMemoryFilePath(atmosConfig)

	// Check if file exists.
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", errUtils.ErrAIProjectMemoryNotFound, filePath)
	}

	// Try to load memory (this validates parsing).
	ctx := context.Background()
	projectMemory, err := manager.Load(ctx)
	if err != nil {
		u.PrintMessage(fmt.Sprintf("❌ Validation failed: %s", err.Error()))
		return err
	}

	// Report validation results.
	u.PrintMessage("✅ Memory file is valid")
	u.PrintMessage(fmt.Sprintf("\nFile: %s", filePath))
	u.PrintMessage(fmt.Sprintf("Sections found: %d", len(projectMemory.Sections)))

	if len(projectMemory.Sections) > 0 {
		u.PrintMessage("\nAvailable sections:")
		for key, section := range projectMemory.Sections {
			u.PrintMessage(fmt.Sprintf("  - %s (%s)", section.Name, key))
		}
	}

	return nil
}

// editMemoryCommand opens memory in editor.
func editMemoryCommand(cmd *cobra.Command, args []string) error {
	log.Debug("Opening memory in editor")

	_, atmosConfig, err := initMemoryManager()
	if err != nil {
		return err
	}

	filePath := getMemoryFilePath(atmosConfig)

	// Check if file exists, create if not.
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		u.PrintMessage("Memory file not found. Creating template...")

		manager, _, err := initMemoryManager()
		if err != nil {
			return err
		}

		ctx := context.Background()
		if err := manager.CreateDefault(ctx); err != nil {
			return fmt.Errorf("failed to create memory file: %w", err)
		}

		u.PrintMessage(fmt.Sprintf("✅ Created %s", filePath))
	}

	// Get editor from environment or use vim.
	_ = viper.BindEnv("editor", "EDITOR")
	editor := viper.GetString("editor")
	if editor == "" {
		editor = "vim"
	}

	u.PrintMessage(fmt.Sprintf("Opening %s in %s...", filepath.Base(filePath), editor))

	// Open editor.
	editorCmd := exec.Command(editor, filePath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	u.PrintMessage("\n✅ Memory file saved")

	return nil
}

// pathMemoryCommand shows memory file path.
func pathMemoryCommand(cmd *cobra.Command, args []string) error {
	log.Debug("Showing memory file path")

	_, atmosConfig, err := initMemoryManager()
	if err != nil {
		return err
	}

	filePath := getMemoryFilePath(atmosConfig)

	u.PrintMessage(filePath)

	return nil
}
