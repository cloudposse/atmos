package user

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
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

	// Parse global flags and build ConfigAndStacksInfo to honor --base-path, --config, --config-path, --profile.
	v := viper.GetViper()
	configAndStacksInfo := flags.BuildConfigAndStacksInfo(cmd, v)

	// Load atmos config.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrInvalidAuthConfig, err)
	}

	// Select aws/user identities.
	selectable, _, err := selectAWSUserIdentities(atmosConfig.Auth.Identities)
	if err != nil {
		return err
	}

	// Guard: ensure there are identities to configure.
	if len(selectable) == 0 {
		return fmt.Errorf("%w: no AWS user identities found in configuration", errUtils.ErrNoIdentitiesAvailable)
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
		ui.Writeln("All credentials are managed by Atmos configuration (atmos.yaml)")
		ui.Writeln("To update credentials, edit your atmos.yaml configuration file")
		if yamlInfo.MfaArn != "" {
			ui.Writef("MFA ARN is also managed by Atmos configuration: %s\n", yamlInfo.MfaArn)
		}
		return nil
	}

	// Resolve the realm the same way pkg/auth/manager.go does so configure writes
	// to the same keyring slot the login resolver reads from. Without this, a user
	// who sets auth.realm or ATMOS_AUTH_REALM would have configure store under one
	// key while login reads another — making the keyring lookup silently miss.
	realmInfo, err := realm.GetRealm(atmosConfig.Auth.Realm, atmosConfig.CliConfigPath)
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrInvalidAuthConfig, err)
	}

	// Prompt and save credentials.
	return promptAndSaveCredentials(yamlInfo, alias, realmInfo.Value)
}

// promptAndSaveCredentials prompts for credentials and saves them to keyring.
func promptAndSaveCredentials(yamlInfo awsUserIdentityInfo, alias, authRealm string) error {
	defer perf.Track(nil, "auth.user.promptAndSaveCredentials")()

	// Prompt user for credentials.
	creds, err := promptForCredentials(yamlInfo)
	if err != nil {
		return err
	}

	// Save credentials to keyring under the resolved realm so the login resolver
	// finds the same entry.
	store := credentials.NewCredentialStore()
	if err := store.Store(alias, creds, authRealm); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrAwsAuth, err)
	}
	ui.Writef("Saved credentials to keyring: %s\n", alias)
	if creds.SessionDuration != "" {
		ui.Writef("Session duration configured: %s\n", creds.SessionDuration)
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
