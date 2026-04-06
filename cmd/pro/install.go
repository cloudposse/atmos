package pro

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/pro/install"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_pro_install.md
var installLongMarkdown string

//go:embed markdown/atmos_pro_install_next_steps.md
var nextStepsMarkdown string

// installCmd scaffolds Atmos Pro configuration files.
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Atmos Pro workflows and configuration",
	Long:  installLongMarkdown,
	Args:  cobra.NoArgs,
	RunE:  runInstall,
}

func init() {
	installCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	installCmd.Flags().BoolP("force", "f", false, "Overwrite existing files")
	installCmd.Flags().Bool("dry-run", false, "Show what would be created without writing files")
}

// resolveInstallPaths loads atmos config and resolves base/stacks paths.
func resolveInstallPaths() (basePath, stacksBasePath string) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return ".", "stacks"
	}
	basePath = atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}
	stacksBasePath = atmosConfig.Stacks.BasePath
	if stacksBasePath == "" {
		stacksBasePath = "stacks"
	}
	return basePath, stacksBasePath
}

func runInstall(cmd *cobra.Command, _ []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	basePath, stacksBasePath := resolveInstallPaths()

	// Prompt for confirmation unless --yes or --dry-run.
	if !dryRun && !yes {
		confirmed, err := flags.PromptForConfirmation(
			"Install Atmos Pro workflows and configuration?",
			false,
		)
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirmed {
			ui.Warning("Installation cancelled")
			return nil
		}
	}

	installer := install.NewInstaller(&install.OSFileWriter{},
		install.WithBasePath(basePath),
		install.WithStacksBasePath(stacksBasePath),
		install.WithForce(force),
	)

	if dryRun {
		reportDryRun(installer.DryRun())
		return nil
	}

	result, err := installer.Install()
	if err != nil {
		return err
	}

	reportResult(result)
	ui.MarkdownMessage(nextStepsMarkdown)

	return nil
}

// reportResult displays the installation results.
func reportResult(result *install.InstallResult) {
	for _, f := range result.CreatedFiles {
		ui.Successf("Created %s", f)
	}
	for _, f := range result.UpdatedFiles {
		ui.Successf("Updated %s", f)
	}
	for _, f := range result.SkippedFiles {
		ui.Warningf("Skipped %s (already exists, use --force to overwrite)", f)
	}
}

// reportDryRun displays what would happen during installation.
func reportDryRun(result *install.InstallResult) {
	ui.Infof("Dry run - no files will be written\n")
	for _, f := range result.CreatedFiles {
		ui.Infof("Would create %s", f)
	}
	for _, f := range result.UpdatedFiles {
		ui.Infof("Would update %s", f)
	}
	for _, f := range result.SkippedFiles {
		ui.Warningf("Would skip %s (already exists)", f)
	}
}
