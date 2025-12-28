package user

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// authUserConfigureCmd prompts for static AWS user credentials and stores them in keyring.
var authUserConfigureCmd = &cobra.Command{
	Use:                "configure",
	Short:              "Configure static AWS user credentials (stored securely in keyring)",
	Long:               `Configure static AWS user credentials (stored securely in keyring)`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE:               executeAuthUserConfigureCommand,
}

func init() {
	defer perf.Track(nil, "auth.user.configure.init")()
	AuthUserCmd.AddCommand(authUserConfigureCmd)
}

// executeAuthUserConfigureCommand is the main execution function for auth user configure.
func executeAuthUserConfigureCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "auth.user.executeAuthUserConfigureCommand")()

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrInvalidAuthConfig, err)
	}

	// Select aws/user identities.
	selectable, _, err := selectAWSUserIdentities(atmosConfig.Auth.Identities)
	if err != nil {
		return err
	}

	// Choose identity.
	choice := selectable[0]
	selector := huh.NewSelect[string]().
		Value(&choice).
		OptionsFunc(func() []huh.Option[string] { return huh.NewOptions(selectable...) }, nil).
		Title("Choose an identity to configure").
		WithTheme(utils.NewAtmosHuhTheme())
	if err := selector.Run(); err != nil {
		return err
	}

	// For AWS User identities, use the identity name directly as alias.
	alias := choice // AWS User identities are standalone, no provider needed.

	// Extract credential information from YAML config.
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

	// Prompt and save credentials.
	return promptAndSaveCredentials(cmd, yamlInfo, alias)
}

// promptAndSaveCredentials prompts for credentials and saves them to keyring.
func promptAndSaveCredentials(cmd *cobra.Command, yamlInfo awsUserIdentityInfo, alias string) error {
	defer perf.Track(nil, "auth.user.promptAndSaveCredentials")()

	// Prompt user for credentials.
	creds, err := promptForCredentials(yamlInfo)
	if err != nil {
		return err
	}

	// Save credentials to keyring.
	store := credentials.NewCredentialStore()
	if err := store.Store(alias, creds); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAwsAuth, err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Saved credentials to keyring: %s\n", alias)
	if creds.SessionDuration != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "Session duration configured: %s\n", creds.SessionDuration)
	}
	return nil
}

// promptForCredentials builds and runs the credential form.
func promptForCredentials(yamlInfo awsUserIdentityInfo) (*authTypes.AWSCredentials, error) {
	defer perf.Track(nil, "auth.user.promptForCredentials")()

	var accessKeyID, secretAccessKey, mfaArn, sessionDuration string
	var formFields []huh.Field

	// Access Key ID.
	formFields = append(formFields, buildCredentialFormField(formFieldConfig{
		YAMLValue:  yamlInfo.AccessKeyID,
		InputTitle: "AWS Access Key ID",
		NoteTitle:  "AWS Access Key ID (managed by Atmos configuration)",
		NoteDesc:   yamlInfo.AccessKeyID,
		IsOptional: false,
	}, &accessKeyID))

	// Secret Access Key.
	formFields = append(formFields, buildCredentialFormField(formFieldConfig{
		YAMLValue:  yamlInfo.SecretAccessKey,
		InputTitle: "AWS Secret Access Key",
		NoteTitle:  "AWS Secret Access Key (managed by Atmos configuration)",
		NoteDesc:   "****** (configured in atmos.yaml)",
		IsPassword: true,
		IsOptional: false,
	}, &secretAccessKey))

	// MFA ARN.
	formFields = append(formFields, buildCredentialFormField(formFieldConfig{
		YAMLValue:  yamlInfo.MfaArn,
		InputTitle: "AWS User MFA ARN (optional)",
		NoteTitle:  "AWS User MFA ARN (managed by Atmos configuration)",
		NoteDesc:   yamlInfo.MfaArn,
		IsOptional: true,
	}, &mfaArn))

	// Session Duration.
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

	form := huh.NewForm(huh.NewGroup(formFields...)).WithTheme(utils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		return nil, err
	}

	return &authTypes.AWSCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		MfaArn:          mfaArn,
		SessionDuration: sessionDuration,
	}, nil
}
