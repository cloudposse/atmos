package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/auth/migrate/awssso"
	"github.com/cloudposse/atmos/pkg/perf"
)

// authMigrateAwsSsoCmd migrates to AWS SSO authentication.
var authMigrateAwsSsoCmd = &cobra.Command{
	Use:   "aws-sso",
	Short: "Migrate to AWS SSO authentication",
	Long:  "Migrate from legacy account-map/iam-roles auth to atmos-managed AWS SSO auth with profiles and identities.",
	RunE:  executeAuthMigrateAwsSso,
}

func executeAuthMigrateAwsSso(cmd *cobra.Command, args []string) error {
	handleHelpRequest(cmd, args)

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")

	defer perf.Track(nil, "cmd.executeAuthMigrateAwsSso")()

	ctx := context.Background()
	fs := &migrate.OSFileSystem{}
	prompter := &migrate.HuhPrompter{}

	// Build migration context (loads config, discovers files).
	migCtx, err := awssso.BuildMigrationContext(ctx, fs, prompter)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrMigrationPrerequisitesNotMet, err)
	}

	// Create steps and coordinator.
	steps := awssso.NewAWSSSOSteps(migCtx, fs)
	coordinator := migrate.NewCoordinator(steps, prompter)

	// Run migration.
	return coordinator.Run(ctx, dryRun, force)
}

func init() {
	authMigrateAwsSsoCmd.Flags().Bool("dry-run", false, "Show migration plan without making changes")
	authMigrateAwsSsoCmd.Flags().Bool("force", false, "Apply all changes without confirmation prompts")
	authMigrateCmd.AddCommand(authMigrateAwsSsoCmd)
}
