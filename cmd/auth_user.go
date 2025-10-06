package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authUserCmd groups user-related auth commands.
var authUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage cloud provider credentials in the local keychain",
	Long: `Store and manage user credentials in the local system keychain.
These credentials are used by Atmos to authenticate with cloud providers
(e.g. AWS IAM). Currently, only AWS IAM user credentials are supported.`,
}

// configure command prompts for static AWS user credentials and stores them in keyring.
var authUserConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure static AWS user credentials (stored securely in keyring)",
	Long:  `Configure static AWS user credentials (stored securely in keyring)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load atmos config
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return errors.Join(errUtils.ErrInvalidAuthConfig, err)
		}

		// Gather identities that use a provider of type aws/user.
		var selectable []string
		if atmosConfig.Auth.Identities == nil {
			return fmt.Errorf("%w: no auth identities configured in atmos.yaml", errUtils.ErrInvalidAuthConfig)
		}
		defaultChoice := ""
		for ident := range atmosConfig.Auth.Identities {
			identity := atmosConfig.Auth.Identities[ident]
			if identity.Kind == "aws/user" {
				selectable = append(selectable, ident)
				if identity.Default && defaultChoice == "" {
					defaultChoice = ident
				}
			}
		}
		if len(selectable) == 0 {
			return fmt.Errorf("%w: no identities configured for provider type 'aws/user'. Define one under auth.identities in atmos.yaml", errUtils.ErrInvalidAuthConfig)
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
						return errUtils.ErrMissingInput
					}
					return nil
				}),
				huh.NewInput().Title("AWS Secret Access Key").Value(&secretAccessKey).EchoMode(huh.EchoModePassword).Validate(func(s string) error {
					if s == "" {
						return errUtils.ErrMissingInput
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

		// Create concrete AWS credentials implementing ICredentials
		creds := &types.AWSCredentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			MfaArn:          mfaArn,
		}

		// Store the credentials
		if err := store.Store(alias, creds); err != nil {
			return errors.Join(errUtils.ErrAwsAuth, err)
		}
		log.Info("Saved credentials to keyring", "alias", alias)
		return nil
	},
}

func init() {
	authCmd.AddCommand(authUserCmd)
	authUserCmd.AddCommand(authUserConfigureCmd)
}
