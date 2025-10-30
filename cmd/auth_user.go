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
	authUtils "github.com/cloudposse/atmos/pkg/auth/utils"
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

		// Check which credentials are already configured in YAML for this identity.
		// If any credential is in YAML, it's managed by Atmos configuration and
		// should not be edited here (should be in version control).
		selectedIdentity := atmosConfig.Auth.Identities[choice]
		var yamlAccessKeyID, yamlSecretAccessKey, yamlMfaArn, yamlSessionDuration string
		if selectedIdentity.Credentials != nil {
			if accessKey, ok := selectedIdentity.Credentials["access_key_id"].(string); ok {
				yamlAccessKeyID = accessKey
			}
			if secretKey, ok := selectedIdentity.Credentials["secret_access_key"].(string); ok {
				yamlSecretAccessKey = secretKey
			}
			if mfa, ok := selectedIdentity.Credentials["mfa_arn"].(string); ok {
				yamlMfaArn = mfa
			}
		}
		// Check if session duration is configured at identity level.
		if selectedIdentity.Session != nil && selectedIdentity.Session.Duration != "" {
			yamlSessionDuration = selectedIdentity.Session.Duration
		}

		// Check if all credentials are managed by Atmos configuration.
		allInYaml := yamlAccessKeyID != "" && yamlSecretAccessKey != ""
		if allInYaml {
			// All credentials are in YAML - nothing to configure in keyring.
			log.Info("All credentials are managed by Atmos configuration (atmos.yaml)", "identity", choice)
			log.Info("To update credentials, edit your atmos.yaml configuration file")
			if yamlMfaArn != "" {
				log.Info("MFA ARN is also managed by Atmos configuration", "mfa_arn", yamlMfaArn)
			}
			return nil
		}

		// Build form fields based on what's already in YAML config.
		var accessKeyID, secretAccessKey, mfaArn, sessionDuration string
		var formFields []huh.Field

		// Access Key ID
		if yamlAccessKeyID != "" {
			// Access Key ID is in YAML config - show it as informational (not editable).
			formFields = append(formFields,
				huh.NewNote().
					Title("AWS Access Key ID (managed by Atmos configuration)").
					Description(yamlAccessKeyID),
			)
			accessKeyID = yamlAccessKeyID
		} else {
			// Access Key ID not in YAML - allow user to enter it (will be stored in keyring).
			formFields = append(formFields,
				huh.NewInput().Title("AWS Access Key ID").Value(&accessKeyID).Validate(func(s string) error {
					if s == "" {
						return errUtils.ErrMissingInput
					}
					return nil
				}),
			)
		}

		// Secret Access Key
		if yamlSecretAccessKey != "" {
			// Secret Access Key is in YAML config - show it as informational (not editable).
			formFields = append(formFields,
				huh.NewNote().
					Title("AWS Secret Access Key (managed by Atmos configuration)").
					Description("****** (configured in atmos.yaml)"),
			)
			secretAccessKey = yamlSecretAccessKey
		} else {
			// Secret Access Key not in YAML - allow user to enter it (will be stored in keyring).
			formFields = append(formFields,
				huh.NewInput().Title("AWS Secret Access Key").Value(&secretAccessKey).EchoMode(huh.EchoModePassword).Validate(func(s string) error {
					if s == "" {
						return errUtils.ErrMissingInput
					}
					return nil
				}),
			)
		}

		// MFA ARN
		if yamlMfaArn != "" {
			// MFA ARN is in YAML config - show it as informational (not editable).
			formFields = append(formFields,
				huh.NewNote().
					Title("AWS User MFA ARN (managed by Atmos configuration)").
					Description(yamlMfaArn),
			)
			mfaArn = yamlMfaArn
		} else {
			// MFA ARN not in YAML - allow user to enter it (will be stored in keyring).
			formFields = append(formFields,
				huh.NewInput().Title("AWS User MFA ARN (optional)").Value(&mfaArn),
			)
		}

		// Session Duration
		if yamlSessionDuration != "" {
			// Session duration is in YAML config - show it as informational (not editable).
			formFields = append(formFields,
				huh.NewNote().
					Title("Session Duration (managed by Atmos configuration)").
					Description(fmt.Sprintf("%s (configured in atmos.yaml)", yamlSessionDuration)),
			)
			sessionDuration = yamlSessionDuration
		} else {
			// Session duration not in YAML - allow user to configure it (will be stored in keyring).
			// Default to "12h" if not already set.
			sessionDuration = "12h"
			formFields = append(formFields,
				huh.NewInput().
					Title("Session Duration (optional, default: 12h)").
					Description("How long before you need to re-enter MFA. Examples: 3600 (seconds), 1h, 12h, 1d, 24h (max 36h with MFA)").
					Value(&sessionDuration).
					Validate(func(s string) error {
						if s == "" {
							return nil // Optional field
						}
						// Validate duration format using flexible parser.
						_, err := authUtils.ParseDurationFlexible(s)
						if err != nil {
							return fmt.Errorf("invalid duration format (use: 3600, 1h, 12h, 1d, etc.)")
						}
						return nil
					}),
			)
		}

		form := huh.NewForm(huh.NewGroup(formFields...)).WithTheme(uiutils.NewAtmosHuhTheme())
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
			SessionDuration: sessionDuration,
		}

		// Store the credentials
		if err := store.Store(alias, creds); err != nil {
			return errors.Join(errUtils.ErrAwsAuth, err)
		}
		log.Info("Saved credentials to keyring", "alias", alias)
		if sessionDuration != "" {
			log.Info("Session duration configured", "duration", sessionDuration)
		}
		return nil
	},
}

func init() {
	authCmd.AddCommand(authUserCmd)
	authUserCmd.AddCommand(authUserConfigureCmd)
}
