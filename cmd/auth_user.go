package cmd

import (
	"fmt"

	"github.com/charmbracelet/huh"
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
			return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrInvalidAuthConfig, err)
		}

		// Select aws/user identities
		selectable, _, err := selectAWSUserIdentities(atmosConfig.Auth.Identities)
		if err != nil {
			return err
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

		// Extract credential information from YAML config
		selectedIdentity := atmosConfig.Auth.Identities[choice]
		yamlInfo := extractAWSUserInfo(selectedIdentity)

		// Check if all credentials are managed by Atmos configuration.
		if yamlInfo.AllInYAML {
			// All credentials are in YAML - nothing to configure in keyring.
			fmt.Fprintln(cmd.ErrOrStderr(), "All credentials are managed by Atmos configuration (atmos.yaml)")
			fmt.Fprintln(cmd.ErrOrStderr(), "To update credentials, edit your atmos.yaml configuration file")
			if yamlInfo.MfaArn != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "MFA ARN is also managed by Atmos configuration: %s\n", yamlInfo.MfaArn)
			}
			return nil
		}

		// Build form fields based on what's already in YAML config.
		var accessKeyID, secretAccessKey, mfaArn, sessionDuration string
		var formFields []huh.Field

		// Access Key ID
		formFields = append(formFields, buildCredentialFormField(formFieldConfig{
			YAMLValue:  yamlInfo.AccessKeyID,
			InputTitle: "AWS Access Key ID",
			NoteTitle:  "AWS Access Key ID (managed by Atmos configuration)",
			NoteDesc:   yamlInfo.AccessKeyID,
			IsOptional: false,
		}, &accessKeyID))

		// Secret Access Key
		formFields = append(formFields, buildCredentialFormField(formFieldConfig{
			YAMLValue:  yamlInfo.SecretAccessKey,
			InputTitle: "AWS Secret Access Key",
			NoteTitle:  "AWS Secret Access Key (managed by Atmos configuration)",
			NoteDesc:   "****** (configured in atmos.yaml)",
			IsPassword: true,
			IsOptional: false,
		}, &secretAccessKey))

		// MFA ARN
		formFields = append(formFields, buildCredentialFormField(formFieldConfig{
			YAMLValue:  yamlInfo.MfaArn,
			InputTitle: "AWS User MFA ARN (optional)",
			NoteTitle:  "AWS User MFA ARN (managed by Atmos configuration)",
			NoteDesc:   yamlInfo.MfaArn,
			IsOptional: true,
		}, &mfaArn))

		// Session Duration
		formFields = append(formFields, buildCredentialFormField(formFieldConfig{
			YAMLValue:      yamlInfo.SessionDuration,
			InputTitle:     "Session Duration (optional, default: 12h)",
			NoteTitle:      "Session Duration (managed by Atmos configuration)",
			NoteDesc:       fmt.Sprintf("%s (configured in atmos.yaml)", yamlInfo.SessionDuration),
			IsOptional:     true,
			DefaultValue:   "12h",
			ValidateFunc:   validateSessionDuration,
			DescriptionMsg: "How long before you need to re-enter MFA. Examples: 3600 (seconds), 1h, 12h, 1d, 24h (max 36h with MFA)",
		}, &sessionDuration))

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
			return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAwsAuth, err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "✓ Saved credentials to keyring: %s\n", alias)
		if sessionDuration != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "✓ Session duration configured: %s\n", sessionDuration)
		}
		return nil
	},
}

func init() {
	authCmd.AddCommand(authUserCmd)
	authUserCmd.AddCommand(authUserConfigureCmd)
}
