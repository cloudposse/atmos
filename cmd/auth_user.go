package cmd

import (
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/internal/auth/authstore"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
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

		// Resolve provider and compute alias <provider>/<identity>
		identity := atmosConfig.Auth.Identities[choice]
		providerName := identity.Via.Provider
		if providerName == "" {
			return fmt.Errorf("identity %s does not specify a provider", choice)
		}
		alias := fmt.Sprintf("%s/%s", providerName, choice)

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

		// Save to keyring using generic store
		store := authstore.NewKeyringAuthStore()
		type userSecret struct {
			AccessKeyID     string    `json:"access_key_id"`
			SecretAccessKey string    `json:"secret_access_key"`
			MfaArn          string    `json:"mfa_arn,omitempty"`
			LastUpdated     time.Time `json:"last_updated"`
		}
		secret := userSecret{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			MfaArn:          mfaArn,
			LastUpdated:     time.Now(),
		}
		if err := store.SetAny(alias, secret); err != nil {
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
