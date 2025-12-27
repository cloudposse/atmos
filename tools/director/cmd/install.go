package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/tools/director/internal/toolmgr"
)

func installCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install required tools at pinned versions",
		Long: `Install tools configured in defaults.yaml at their pinned versions.

This command downloads and installs tools (like atmos) to the local cache
directory, ensuring reproducible demo generation.

Tools are only downloaded if they're not already installed at the correct
version (unless --force is specified).`,
		Example: `
# Install all configured tools
director install

# Force reinstall even if already at correct version
director install --force
`,
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()

			demosDir, err := findDemosDir()
			if err != nil {
				return err
			}

			// Load tools configuration.
			toolsConfig, err := toolmgr.LoadToolsConfig(demosDir)
			if err != nil {
				return fmt.Errorf("failed to load tools config: %w", err)
			}

			if toolsConfig == nil {
				c.Println("No tools configured in defaults.yaml")
				return nil
			}

			mgr := toolmgr.New(toolsConfig, demosDir)

			// Install atmos if configured.
			if toolsConfig.Atmos != nil {
				if force {
					// Clear cache to force reinstall.
					if err := mgr.LoadCache(); err != nil {
						return err
					}
				}

				path, err := mgr.EnsureInstalled(ctx, "atmos")
				if err != nil {
					return fmt.Errorf("failed to install atmos: %w", err)
				}

				version, _ := mgr.Version("atmos")
				c.Printf("✓ atmos v%s installed at %s\n", version, path)
			}

			// Future: install other tools here.

			c.Println("\nAll tools installed successfully")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force reinstall even if already at correct version")

	return cmd
}
