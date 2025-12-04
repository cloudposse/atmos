package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// MaxAssumeRootDuration is the maximum session duration for AssumeRoot (15 minutes per AWS limit).
	maxAssumeRootDuration = 900
	// TaskPolicyArnPrefix is the required prefix for all root task policy ARNs.
	taskPolicyArnPrefix = "arn:aws:iam::aws:policy/root-task/"
)

// Supported AWS-managed root task policies.
var supportedTaskPolicies = []string{
	"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
	"arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword",
	"arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials",
	"arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy",
	"arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy",
}

// accountIDPattern validates 12-digit AWS account IDs.
var accountIDPattern = regexp.MustCompile(`^\d{12}$`)

// assumeRootIdentity implements AWS assume root identity for centralized root access.
type assumeRootIdentity struct {
	name             string
	config           *schema.Identity
	region           string
	targetPrincipal  string // Target member account ID (12-digit).
	taskPolicyArn    string // AWS-managed task policy ARN.
	manager          types.AuthManager
	rootProviderName string
}

// NewAssumeRootIdentity creates a new AWS assume root identity.
func NewAssumeRootIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "aws.NewAssumeRootIdentity")()

	if name == "" {
		return nil, fmt.Errorf("%w: identity name is empty", errUtils.ErrInvalidIdentityConfig)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidIdentityConfig)
	}
	if config.Kind != types.ProviderKindAWSAssumeRoot {
		return nil, fmt.Errorf("%w: invalid identity kind for assume root: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	return &assumeRootIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *assumeRootIdentity) Kind() string {
	return types.ProviderKindAWSAssumeRoot
}

// Validate validates the identity configuration.
func (i *assumeRootIdentity) Validate() error {
	defer perf.Track(nil, "aws.assumeRootIdentity.Validate")()

	if i.config.Principal == nil {
		return i.missingPrincipalError()
	}

	if err := i.validateTargetPrincipal(); err != nil {
		return err
	}

	if err := i.validateTaskPolicyArn(); err != nil {
		return err
	}

	// Extract optional region.
	if region, ok := i.config.Principal["region"].(string); ok {
		i.region = region
	}

	return nil
}

func (i *assumeRootIdentity) missingPrincipalError() error {
	return errUtils.Build(errUtils.ErrMissingPrincipal).
		WithExplanationf("Identity '%s' requires principal configuration", i.name).
		WithHint("Add 'principal' field with 'target_principal' and 'task_policy_arn' to the identity configuration").
		WithContext(logKeyIdentity, i.name).
		WithExitCode(2).
		Err()
}

func (i *assumeRootIdentity) validateTargetPrincipal() error {
	targetPrincipal, ok := i.config.Principal["target_principal"].(string)
	if !ok || targetPrincipal == "" {
		return errUtils.Build(errUtils.ErrMissingPrincipal).
			WithExplanationf("Missing 'target_principal' configuration for identity '%s'", i.name).
			WithHint("Add 'target_principal' field with the 12-digit member account ID").
			WithHint("Example: principal: { target_principal: '123456789012' }").
			WithContext(logKeyIdentity, i.name).
			WithExitCode(2).
			Err()
	}

	if !accountIDPattern.MatchString(targetPrincipal) {
		return errUtils.Build(errUtils.ErrInvalidIdentityConfig).
			WithExplanationf("Invalid 'target_principal' format for identity '%s'", i.name).
			WithHint("target_principal must be a 12-digit AWS account ID").
			WithHint(fmt.Sprintf("Provided value: '%s'", targetPrincipal)).
			WithContext(logKeyIdentity, i.name).
			WithContext("target_principal", targetPrincipal).
			WithExitCode(2).
			Err()
	}
	i.targetPrincipal = targetPrincipal
	return nil
}

func (i *assumeRootIdentity) validateTaskPolicyArn() error {
	taskPolicyArn, ok := i.config.Principal["task_policy_arn"].(string)
	if !ok || taskPolicyArn == "" {
		return errUtils.Build(errUtils.ErrMissingPrincipal).
			WithExplanationf("Missing 'task_policy_arn' configuration for identity '%s'", i.name).
			WithHint("Add 'task_policy_arn' field with an AWS-managed root task policy ARN").
			WithHint("Supported policies: "+strings.Join(supportedTaskPolicies, ", ")).
			WithContext(logKeyIdentity, i.name).
			WithExitCode(2).
			Err()
	}

	if !strings.HasPrefix(taskPolicyArn, taskPolicyArnPrefix) {
		return errUtils.Build(errUtils.ErrInvalidIdentityConfig).
			WithExplanationf("Invalid 'task_policy_arn' format for identity '%s'", i.name).
			WithHint("task_policy_arn must be an AWS-managed root task policy").
			WithHint("Supported policies: "+strings.Join(supportedTaskPolicies, ", ")).
			WithContext(logKeyIdentity, i.name).
			WithContext("task_policy_arn", taskPolicyArn).
			WithExitCode(2).
			Err()
	}
	i.taskPolicyArn = taskPolicyArn
	return nil
}

// Authenticate performs authentication using sts:AssumeRoot.
func (i *assumeRootIdentity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.assumeRootIdentity.Authenticate")()

	// Validate identity configuration.
	if err := i.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid assume root identity: %w", errUtils.ErrInvalidIdentityConfig, err)
	}

	// AssumeRoot requires AWS credentials (cannot use OIDC directly).
	awsBase, ok := baseCreds.(*types.AWSCredentials)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrInvalidIdentityConfig).
			WithExplanationf("Invalid credentials type for assume-root identity '%s'", i.name).
			WithHint("Base credentials must be AWS credentials from a permission set or assume-role identity").
			WithHint("Verify the authentication chain is configured correctly in atmos.yaml").
			WithContext(logKeyIdentity, i.name).
			WithContext("target_principal", i.targetPrincipal).
			WithExitCode(2).
			Err()
	}

	// Create STS client with base credentials.
	stsClient, err := i.newSTSClient(ctx, awsBase)
	if err != nil {
		return nil, err
	}

	// Build AssumeRoot input.
	assumeRootInput := i.buildAssumeRootInput()

	// Call AssumeRoot.
	result, err := stsClient.AssumeRoot(ctx, assumeRootInput)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanationf("Failed to assume root on account '%s'", i.targetPrincipal).
			WithHint("Verify the target account is a member of your organization").
			WithHint("Ensure centralized root access is enabled for the target account").
			WithHint("Check that your permission set has sts:AssumeRoot permission").
			WithHint(fmt.Sprintf("Using task policy: %s", i.taskPolicyArn)).
			WithContext(logKeyIdentity, i.name).
			WithContext("target_principal", i.targetPrincipal).
			WithContext("task_policy_arn", i.taskPolicyArn).
			WithContext("region", i.region).
			WithExitCode(1).
			Err()
	}

	return i.toAWSCredentials(result)
}

// newSTSClient creates an STS client using the base credentials and configured region.
func (i *assumeRootIdentity) newSTSClient(ctx context.Context, awsBase *types.AWSCredentials) (*sts.Client, error) {
	client, resolvedRegion, err := NewSTSClientWithCredentials(ctx, awsBase, i.region, i.config)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %w", errUtils.ErrInvalidIdentityConfig, err)
	}
	i.region = resolvedRegion
	return client, nil
}

// buildAssumeRootInput constructs the STS AssumeRootInput.
func (i *assumeRootIdentity) buildAssumeRootInput() *sts.AssumeRootInput {
	input := &sts.AssumeRootInput{
		TargetPrincipal: aws.String(i.targetPrincipal),
		TaskPolicyArn: &ststypes.PolicyDescriptorType{
			Arn: aws.String(i.taskPolicyArn),
		},
	}

	// Add optional duration.
	input.DurationSeconds = i.parseDurationSeconds()

	return input
}

// parseDurationSeconds parses the duration from principal config and returns capped seconds.
func (i *assumeRootIdentity) parseDurationSeconds() *int32 {
	durationStr, ok := i.config.Principal[principalDurationKey].(string)
	if !ok || durationStr == "" {
		return nil
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		log.Warn("Invalid duration specified for assume root", principalDurationKey, durationStr)
		return nil
	}

	seconds := int32(duration.Seconds())
	if seconds > maxAssumeRootDuration {
		log.Warn("Duration exceeds maximum for AssumeRoot (900s), using maximum", principalDurationKey, durationStr)
		seconds = maxAssumeRootDuration
	}
	return aws.Int32(seconds)
}

// toAWSCredentials converts STS AssumeRoot output to AWSCredentials with validation.
func (i *assumeRootIdentity) toAWSCredentials(result *sts.AssumeRootOutput) (types.ICredentials, error) {
	if result == nil || result.Credentials == nil {
		return nil, fmt.Errorf("%w: STS returned empty credentials", errUtils.ErrAuthenticationFailed)
	}

	expiration := ""
	if result.Credentials.Expiration != nil {
		expiration = result.Credentials.Expiration.Format(time.RFC3339)
	}

	finalRegion := i.region
	if finalRegion == "" {
		finalRegion = defaultAWSRegion
	}

	return &types.AWSCredentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
		Region:          finalRegion,
		Expiration:      expiration,
	}, nil
}

// Environment returns environment variables for this identity.
func (i *assumeRootIdentity) Environment() (map[string]string, error) {
	defer perf.Track(nil, "aws.assumeRootIdentity.Environment")()

	env := make(map[string]string)

	// Get root provider name for file storage.
	providerName, err := i.resolveRootProviderName()
	if err != nil {
		return nil, err
	}

	// Get AWS file environment variables.
	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return nil, errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}
	awsEnvVars := awsFileManager.GetEnvironmentVariables(providerName, i.name)

	// Convert to map format.
	for _, envVar := range awsEnvVars {
		env[envVar.Key] = envVar.Value
	}

	// Add environment variables from identity config.
	for _, envVar := range i.config.Env {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
func (i *assumeRootIdentity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.assumeRootIdentity.PrepareEnvironment")()

	// Get root provider name for file storage.
	providerName, err := i.resolveRootProviderName()
	if err != nil {
		return environ, fmt.Errorf("failed to get provider name: %w", err)
	}

	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return environ, fmt.Errorf("failed to create AWS file manager: %w", err)
	}

	credentialsFile := awsFileManager.GetCredentialsPath(providerName)
	configFile := awsFileManager.GetConfigPath(providerName)

	// Get region from identity if available.
	region := i.region

	// Use shared AWS environment preparation helper.
	return awsCloud.PrepareEnvironment(environ, i.name, credentialsFile, configFile, region), nil
}

// GetProviderName extracts the provider name from the identity configuration.
func (i *assumeRootIdentity) GetProviderName() (string, error) {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	if i.config.Via != nil && i.config.Via.Identity != "" {
		return i.config.Via.Identity, nil
	}
	return "", fmt.Errorf("%w: assume root identity %q has no valid via configuration", errUtils.ErrInvalidIdentityConfig, i.name)
}

// resolveRootProviderName resolves the root provider name for file storage.
func (i *assumeRootIdentity) resolveRootProviderName() (string, error) {
	// Try manager first (available after PostAuthenticate).
	if i.manager != nil {
		if providerName := i.manager.GetProviderForIdentity(i.name); providerName != "" {
			return providerName, nil
		}
	}

	// Fall back to cached value or config.
	return i.getRootProviderFromVia()
}

// getRootProviderFromVia gets the root provider name using available information.
func (i *assumeRootIdentity) getRootProviderFromVia() (string, error) {
	// First try cached value set during PostAuthenticate.
	if i.rootProviderName != "" {
		return i.rootProviderName, nil
	}

	// Fall back to via.provider from config (works for single-hop chains).
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}

	// Can't determine root provider.
	return "", fmt.Errorf("%w: cannot determine root provider for identity %q before authentication", errUtils.ErrInvalidAuthConfig, i.name)
}

// SetManagerAndProvider sets the manager and root provider name on the identity.
func (i *assumeRootIdentity) SetManagerAndProvider(manager types.AuthManager, rootProviderName string) {
	i.manager = manager
	i.rootProviderName = rootProviderName
}

// PostAuthenticate sets up AWS files and populates auth context after authentication.
func (i *assumeRootIdentity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	defer perf.Track(nil, "aws.assumeRootIdentity.PostAuthenticate")()

	// Guard against nil parameters.
	if params == nil {
		return fmt.Errorf("%w: PostAuthenticate parameters cannot be nil", errUtils.ErrInvalidAuthConfig)
	}
	if params.Credentials == nil {
		return fmt.Errorf("%w: credentials are required", errUtils.ErrInvalidAuthConfig)
	}

	// Store manager reference and root provider name for resolving in file operations.
	i.manager = params.Manager
	i.rootProviderName = params.ProviderName

	// Setup AWS files using shared AWS cloud package.
	if err := awsCloud.SetupFiles(params.ProviderName, params.IdentityName, params.Credentials, ""); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	// Populate auth context (single source of truth for runtime credentials).
	if err := awsCloud.SetAuthContext(&awsCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: params.ProviderName,
		IdentityName: params.IdentityName,
		Credentials:  params.Credentials,
		BasePath:     "",
	}); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	// Derive environment variables from auth context for spawned processes.
	if err := awsCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	return nil
}

// CredentialsExist checks if credentials exist for this identity.
func (i *assumeRootIdentity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "aws.assumeRootIdentity.CredentialsExist")()

	// Get root provider name for file storage.
	providerName, err := i.resolveRootProviderName()
	if err != nil {
		return false, err
	}

	mgr, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return false, err
	}

	credPath := mgr.GetCredentialsPath(providerName)

	// Load and parse the credentials file to verify the identity section exists.
	cfg, err := awsCloud.LoadINIFile(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("load credentials file: %w", err)
	}

	// Check if this identity's section exists in the credentials file.
	sec, err := cfg.GetSection(i.name)
	if err != nil {
		return false, nil
	}

	// Verify the section has actual credential keys.
	if strings.TrimSpace(sec.Key("aws_access_key_id").String()) == "" {
		return false, nil
	}

	return true, nil
}

// LoadCredentials loads AWS credentials from files using environment variables.
func (i *assumeRootIdentity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.assumeRootIdentity.LoadCredentials")()

	// Get environment variables that specify where credentials are stored.
	env, err := i.Environment()
	if err != nil {
		return nil, fmt.Errorf("failed to get environment variables: %w", err)
	}

	// Load credentials from files using AWS SDK.
	creds, err := loadAWSCredentialsFromEnvironment(ctx, env)
	if err != nil {
		return nil, err
	}

	return creds, nil
}

// Logout removes identity-specific credential storage.
func (i *assumeRootIdentity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.assumeRootIdentity.Logout")()

	log.Debug("Logout assume-root identity", logKeyIdentity, i.name, "provider", i.rootProviderName)

	basePath := ""

	fileManager, err := awsCloud.NewAWSFileManager(basePath)
	if err != nil {
		log.Debug("Failed to create file manager for logout", logKeyIdentity, i.name, "error", err)
		return fmt.Errorf("failed to create AWS file manager: %w", err)
	}

	// Remove this identity's profile from the provider's config files.
	if err := fileManager.DeleteIdentity(ctx, i.rootProviderName, i.name); err != nil {
		log.Debug("Failed to delete identity files", logKeyIdentity, i.name, "error", err)
		return fmt.Errorf("failed to delete identity files: %w", err)
	}

	log.Debug("Successfully deleted assume-root identity", "identity", i.name)
	return nil
}

// IsSupportedTaskPolicy checks if a task policy ARN is in the list of known supported policies.
func IsSupportedTaskPolicy(arn string) bool {
	for _, policy := range supportedTaskPolicies {
		if policy == arn {
			return true
		}
	}
	return false
}

// GetSupportedTaskPolicies returns the list of supported AWS-managed root task policies.
func GetSupportedTaskPolicies() []string {
	result := make([]string, len(supportedTaskPolicies))
	copy(result, supportedTaskPolicies)
	return result
}
