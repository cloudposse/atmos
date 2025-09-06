package cmd

import (
	"fmt"

	"github.com/charmbracelet/huh"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/internal/auth/credentials"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
)

// authUserCmd groups user-related auth commands.
var authUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage aws user credentials for atmos auth",
}

// configure command prompts for static AWS user credentials and stores them in keyring.
var authUserConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure static AWS user credentials (stored securely in keyring)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load atmos config
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		// Gather identities that use a provider of type aws/user
		var selectable []string
		for ident := range atmosConfig.Auth.Identities {
			identity := atmosConfig.Auth.Identities[ident]
			if identity.Kind == "aws/user" {
				selectable = append(selectable, ident)
			}
		}
		if len(selectable) == 0 {
			return fmt.Errorf("no identities configured for provider type 'aws/user'. Define one under auth.providers and auth.identities in atmos.yaml")
		}

		// Choose identity
		choice := selectable[0]
		selector := huh.NewSelect[string]().
			Value(&choice).
			OptionsFunc(func() []huh.Option[string] { return huh.NewOptions(selectable...) }, nil).
			Title("Choose an identity to configure").
			WithTheme(uiutils.NewAtmosHuhTheme())
		if err := selector.Run(); err != nil {
			return err
		}

		// For AWS User identities, use the identity name directly as alias
		alias := choice // AWS User identities are standalone, no provider needed

		// Prompt for credentials
		var accessKeyID, secretAccessKey, mfaArn string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title("AWS Access Key ID").Value(&accessKeyID).Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("required")
					}
					return nil
				}),
				huh.NewInput().Title("AWS Secret Access Key").Value(&secretAccessKey).Password(true).Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("required")
					}
					return nil
				}),
				huh.NewInput().Title("AWS User MFA ARN (optional)").Value(&mfaArn),
			),
		).WithTheme(uiutils.NewAtmosHuhTheme())
		if err := form.Run(); err != nil {
			return err
		}

		// Save to keyring using schema.Credentials format
		store := credentials.NewCredentialStore()

		// Create credentials in the proper schema format with AWS wrapper
		creds := &schema.Credentials{
			AWS: &schema.AWSCredentials{
				AccessKeyID:     accessKeyID,
				SecretAccessKey: secretAccessKey,
				MfaArn:          mfaArn,
			},
		}

		// Store the credentials
		if err := store.Store(alias, creds); err != nil {
			return err
		}
		log.Info("Saved credentials to keyring", "alias", alias)
		return nil
	},
}

func init() {
	authCmd.AddCommand(authUserCmd)
	authUserCmd.AddCommand(authUserConfigureCmd)
}
