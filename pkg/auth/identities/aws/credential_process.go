package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// The synthetic provider name under which the
	// Atmos-managed credential files of aws/credential-process identities are written.
	awsCredentialProcessProviderName = types.ProviderNameAWSCredentialProcess

	// How long cached helper credentials must remain valid to be
	// reused instead of running the helper again. It matches the auth manager's credential
	// validity buffer, so a chain never re-authenticates the root only to receive the same
	// nearly-expired credentials back.
	credentialProcessReuseBuffer = 15 * time.Minute

	// Points users with IAM-user access keys to the right kind.
	credentialProcessAWSUserHint = "Use an identity with kind aws/user for IAM user access keys, MFA, and session tokens."
)

// processRetrieveFunc runs an AWS credential_process command and returns the credentials it prints.
// It is a struct field on the identity so tests can inject a fake instead of spawning a process.
type processRetrieveFunc func(ctx context.Context, identityName, command string) (*types.AWSCredentials, error)

// credentialProcessIdentity implements the aws/credential-process identity.
//
// It is a standalone identity: its credentials come from running an AWS `credential_process`
// command (the helper), not from an upstream Atmos provider. The helper's output is used as-is
// (no STS call, no MFA prompt) and cached in the Atmos-managed AWS files while it is unexpired.
// It never touches the keyring.
type credentialProcessIdentity struct {
	name   string
	config *schema.Identity
	realm  string // Credential isolation realm set by auth manager.

	// retrieve runs the credential_process command. Defaults to awsCloud.RetrieveProcessCredentials.
	retrieve processRetrieveFunc
}

// NewCredentialProcessIdentity creates a new aws/credential-process identity.
func NewCredentialProcessIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "aws.NewCredentialProcessIdentity")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidAuthConfig, name)
	}
	if config.Kind != types.IdentityKindAWSCredentialProcess {
		return nil, fmt.Errorf("%w: invalid identity kind for aws/credential-process: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	return &credentialProcessIdentity{
		name:     name,
		config:   config,
		retrieve: defaultProcessRetrieve,
	}, nil
}

// defaultProcessRetrieve runs the command through the real AWS credential_process machinery.
func defaultProcessRetrieve(ctx context.Context, identityName, command string) (*types.AWSCredentials, error) {
	return awsCloud.RetrieveProcessCredentials(ctx, identityName, command)
}

// Kind returns the identity kind.
func (i *credentialProcessIdentity) Kind() string {
	return types.IdentityKindAWSCredentialProcess
}

// SetRealm sets the credential isolation realm for this identity.
func (i *credentialProcessIdentity) SetRealm(realm string) {
	i.realm = realm
}

// GetProviderName returns the provider name for this identity.
// Identities of kind aws/credential-process are standalone and always return "aws-credential-process".
func (i *credentialProcessIdentity) GetProviderName() (string, error) {
	return awsCredentialProcessProviderName, nil
}

// IsStandalone reports that aws/credential-process identities authenticate without an upstream
// provider step. Part of the types.StandaloneIdentity interface.
func (i *credentialProcessIdentity) IsStandalone() bool { return true }

// AuthenticateStandalone authenticates the identity directly, without upstream credentials.
// Part of the types.StandaloneIdentity interface.
func (i *credentialProcessIdentity) AuthenticateStandalone(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.credentialProcessIdentity.AuthenticateStandalone")()

	log.Debug("Authenticating AWS credential-process identity directly", logKeyIdentity, i.name)

	creds, err := i.Authenticate(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: AWS credential-process identity %q authentication failed: %w", errUtils.ErrAuthenticationFailed, i.name, err)
	}

	return creds, nil
}

// Authenticate returns unexpired credentials from the Atmos-managed files when present, and
// otherwise runs the credential_process command and caches its output in those files.
// Credentials without an expiration are never reused: the helper owns their lifecycle.
func (i *credentialProcessIdentity) Authenticate(ctx context.Context, _ types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.credentialProcessIdentity.Authenticate")()

	if err := i.Validate(); err != nil {
		return nil, err
	}

	if existing := i.reusableFileCredentials(ctx); existing != nil {
		log.Debug("Using existing valid credential_process credentials from AWS files", logKeyIdentity, i.name)
		return existing, nil
	}

	creds, err := i.retrieve(ctx, i.name, i.command())
	if err != nil {
		return nil, err
	}
	if creds == nil {
		return nil, fmt.Errorf("%w: credential_process returned no credentials for identity %q", errUtils.ErrCredentialProcessInvalidOutput, i.name)
	}

	if creds.Region == "" {
		creds.Region = i.resolveRegion()
	}

	if err := i.writeAWSFiles(creds); err != nil {
		return nil, fmt.Errorf("%w: failed to write AWS files: %w", errUtils.ErrAwsAuth, err)
	}

	return creds, nil
}

// reusableFileCredentials returns the cached file credentials when they carry an expiration
// more than credentialProcessReuseBuffer away, and nil otherwise (including when the files are
// missing or unreadable).
func (i *credentialProcessIdentity) reusableFileCredentials(ctx context.Context) *types.AWSCredentials {
	existing, err := i.loadFileCredentials(ctx)
	if err != nil {
		log.Debug("No existing credential_process credentials found, running command", logKeyIdentity, i.name, "error", err)
		return nil
	}
	if existing.Expiration == "" {
		log.Debug("Cached credential_process credentials have no expiration, running command", logKeyIdentity, i.name)
		return nil
	}
	expTime, err := existing.GetExpiration()
	if err != nil || expTime == nil || !expTime.After(time.Now().Add(credentialProcessReuseBuffer)) {
		log.Debug("Cached credential_process credentials are expired or expire soon, running command",
			logKeyIdentity, i.name, "required_buffer", credentialProcessReuseBuffer)
		return nil
	}
	return existing
}

// command returns the configured credential_process command (empty when unset).
func (i *credentialProcessIdentity) command() string {
	command, _ := i.config.Credentials["credential_process"].(string)
	return strings.TrimSpace(command)
}

// resolveRegion returns the configured region or the default one.
func (i *credentialProcessIdentity) resolveRegion() string {
	if r, ok := i.config.Credentials["region"].(string); ok && r != "" {
		return r
	}
	return defaultRegion
}

// writeAWSFiles writes credentials to the Atmos-managed AWS files under the
// "aws-credential-process" provider directory, including the expiration comment.
func (i *credentialProcessIdentity) writeAWSFiles(creds *types.AWSCredentials) error {
	return awsCloud.SetupFiles(awsCredentialProcessProviderName, i.name, creds, "", i.realm)
}

// Validate validates the identity configuration.
// It requires credentials.credential_process and rejects settings that belong to aws/user.
func (i *credentialProcessIdentity) Validate() error {
	defer perf.Track(nil, "aws.credentialProcessIdentity.Validate")()

	if i.config.Via != nil {
		return errUtils.Build(fmt.Errorf("%w: aws/credential-process identity %q must not define via; its credentials come from the credential_process command",
			errUtils.ErrInvalidIdentityConfig, i.name)).
			WithHint("Remove the via section. Other identities can chain from this one with via.identity.").
			WithContext("identity", i.name).
			Err()
	}

	for _, field := range []string{"access_key_id", "secret_access_key", "mfa_arn"} {
		if value, ok := i.config.Credentials[field]; ok && value != nil && value != "" {
			return errUtils.Build(fmt.Errorf("%w: aws/credential-process identity %q must not define credentials.%s",
				errUtils.ErrInvalidIdentityConfig, i.name, field)).
				WithHint(credentialProcessAWSUserHint).
				WithContext("identity", i.name).
				Err()
		}
	}

	if i.command() == "" {
		return errUtils.Build(fmt.Errorf("%w: aws/credential-process identity %q requires credentials.credential_process",
			errUtils.ErrInvalidIdentityConfig, i.name)).
			WithHint("Set credentials.credential_process to the command that prints AWS credentials as JSON, for example the command from a credential_process line in ~/.aws/config.").
			WithContext("identity", i.name).
			Err()
	}

	return nil
}

// Environment returns the environment variables pointing at this identity's Atmos-managed files.
func (i *credentialProcessIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	awsFileManager, err := awsCloud.NewAWSFileManager("", i.realm)
	if err != nil {
		return nil, errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}
	for _, envVar := range awsFileManager.GetEnvironmentVariables(awsCredentialProcessProviderName, i.name) {
		env[envVar.Key] = envVar.Value
	}

	// Include region ONLY if explicitly configured (not the default fallback).
	if r, ok := i.config.Credentials["region"].(string); ok && r != "" {
		env["AWS_REGION"] = r
		env["AWS_DEFAULT_REGION"] = r
	}

	for _, envVar := range i.config.Env {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// Paths returns credential files/directories used by this identity.
func (i *credentialProcessIdentity) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment prepares environment variables for external processes using the shared
// AWS helper (credential files, profile, region, IMDS disabled).
func (i *credentialProcessIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.credentialProcessIdentity.PrepareEnvironment")()

	awsFileManager, err := awsCloud.NewAWSFileManager("", i.realm)
	if err != nil {
		return environ, fmt.Errorf("failed to create AWS file manager: %w", err)
	}

	credentialsFile := awsFileManager.GetCredentialsPath(awsCredentialProcessProviderName)
	configFile := awsFileManager.GetConfigPath(awsCredentialProcessProviderName)

	return awsCloud.PrepareEnvironment(environ, i.name, credentialsFile, configFile, i.resolveRegion()), nil
}

// PostAuthenticate sets up AWS files and populates the auth context after authentication.
// The endpoint (spec.endpoint_url) is resolved from the identity config by SetAuthContext.
func (i *credentialProcessIdentity) PostAuthenticate(_ context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "aws.credentialProcessIdentity.PostAuthenticate")()

	if params == nil {
		return fmt.Errorf("%w: PostAuthenticate parameters cannot be nil", errUtils.ErrInvalidAuthConfig)
	}
	if params.Credentials == nil {
		return fmt.Errorf("%w: credentials are required", errUtils.ErrInvalidAuthConfig)
	}

	// Always use the fixed provider name to avoid path drift.
	providerName := awsCredentialProcessProviderName

	if err := awsCloud.SetupFiles(providerName, i.name, params.Credentials, "", params.Realm); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	if err := awsCloud.SetAuthContext(&awsCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: providerName,
		IdentityName: i.name,
		Credentials:  params.Credentials,
		BasePath:     "",
		Manager:      params.Manager,
		Realm:        params.Realm,
	}); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	if err := awsCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	return nil
}

// CredentialsExist checks whether the Atmos-managed credentials file has a section for this identity.
func (i *credentialProcessIdentity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "aws.credentialProcessIdentity.CredentialsExist")()

	mgr, err := awsCloud.NewAWSFileManager("", i.realm)
	if err != nil {
		return false, err
	}

	cfg, err := awsCloud.LoadINIFile(mgr.GetCredentialsPath(awsCredentialProcessProviderName))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("load credentials file: %w", err)
	}

	sec, err := cfg.GetSection(i.name)
	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(sec.Key("aws_access_key_id").String()) != "", nil
}

// LoadCredentials loads credentials from the Atmos-managed files. It is read-only and never runs
// the credential_process command.
func (i *credentialProcessIdentity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.credentialProcessIdentity.LoadCredentials")()

	creds, err := i.loadFileCredentials(ctx)
	if err != nil {
		return nil, err
	}
	return creds, nil
}

// loadFileCredentials reads credentials from the files without any side effects.
func (i *credentialProcessIdentity) loadFileCredentials(ctx context.Context) (*types.AWSCredentials, error) {
	env, err := i.Environment()
	if err != nil {
		return nil, fmt.Errorf("failed to get environment variables: %w", err)
	}
	return loadAWSCredentialsFromEnvironment(ctx, env)
}

// Logout removes this identity's sections from the Atmos-managed credential files.
func (i *credentialProcessIdentity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.credentialProcessIdentity.Logout")()

	fileManager, err := awsCloud.NewAWSFileManager("", i.realm)
	if err != nil {
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	// DeleteIdentity removes only this identity's sections, preserving other identities' files.
	if err := fileManager.DeleteIdentity(ctx, awsCredentialProcessProviderName, i.name); err != nil {
		log.Debug("Failed to delete AWS files for credential-process identity", logKeyIdentity, i.name, "error", err)
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	log.Debug("Deleted AWS files for credential-process identity", logKeyIdentity, i.name)
	return nil
}
