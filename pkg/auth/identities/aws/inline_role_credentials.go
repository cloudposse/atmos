package aws

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// The pinned AWS SDK (config v1.33) reads its credential chain inputs with os.Getenv and offers no way to
// pass another environment. The base credentials of an inline role must come from the invocation
// environment, and changing the process environment would leak into concurrent invocations. So the chain is
// assembled here, in the SDK's order, from the invocation environment alone: static keys, a web identity
// token file, the shared profile, container credentials, then instance metadata. Only the profile sources
// that the shared files fully describe (keys, web identity, SSO, login, and credential_process) are left to
// the SDK, with every input that selects them passed as an explicit option.

const (
	credentialSourceEnvironment = "Environment"
	credentialSourceContainer   = "EcsContainer"
	credentialSourceIMDS        = "Ec2InstanceMetadata"

	defaultProfileName        = "default"
	ecsContainerEndpoint      = "http://169.254.170.2"
	imdsDefaultIPv4Endpoint   = "http://169.254.169.254"
	imdsDefaultIPv6Endpoint   = "http://[fd00:ec2::254]"
	containerCredentialExpiry = 5 * time.Minute
	// The SDK passes duration_seconds on only when it exceeds 15 whole minutes; shorter values use the STS default.
	minimumProfileRoleDuration = 16 * time.Minute
)

// containerHosts are the ECS and EKS container credential hosts that plain HTTP may reach.
var containerHosts = []net.IP{net.ParseIP("169.254.170.2"), net.ParseIP("169.254.170.23"), net.ParseIP("fd00:ec2::23")}

// errNoInvocationCredentials reports an invocation environment that selects a source with nothing in it.
var errNoInvocationCredentials = errors.New("the invocation environment provides no credentials for the selected source")

// baseCredentials resolves the credentials that sign the AssumeRole request.
func baseCredentials(ctx context.Context, env map[string]string) (*types.AWSCredentials, error) {
	region := cmp.Or(env["AWS_REGION"], env["AWS_DEFAULT_REGION"])
	provider, profileRegion, err := baseCredentialsProvider(ctx, env, region)
	if err != nil {
		return nil, fmt.Errorf("%w: no AWS credentials to assume a role with: %w", errUtils.ErrAuthenticationFailed, err)
	}
	retrieved, err := aws.NewCredentialsCache(provider).Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: no AWS credentials to assume a role with: %w", errUtils.ErrAuthenticationFailed, err)
	}
	return &types.AWSCredentials{
		AccessKeyID:     retrieved.AccessKeyID,
		SecretAccessKey: retrieved.SecretAccessKey,
		SessionToken:    retrieved.SessionToken,
		Region:          cmp.Or(region, profileRegion),
	}, nil
}

// baseCredentialsProvider selects the first source of the SDK default chain that the invocation
// environment configures. It also returns the region of the profile it read, if any.
func baseCredentialsProvider(ctx context.Context, env map[string]string, region string) (aws.CredentialsProvider, string, error) {
	if key, secret := env["AWS_ACCESS_KEY_ID"], env["AWS_SECRET_ACCESS_KEY"]; key != "" && secret != "" {
		return staticEnvCredentials(env), "", nil
	}
	if tokenFile := env["AWS_WEB_IDENTITY_TOKEN_FILE"]; tokenFile != "" {
		provider, err := envWebIdentityCredentials(env, region, tokenFile)
		return provider, "", err
	}
	profile, err := loadInvocationProfile(ctx, env)
	if err != nil {
		return nil, "", err
	}
	if profile == nil {
		provider, err := containerOrInstanceCredentials(env)
		return provider, "", err
	}
	region = cmp.Or(region, profile.Region)
	chain := profileChain(profile)
	if root := chain[len(chain)-1]; !profileSourceNeedsEnvironment(root) {
		provider, err := sdkProfileCredentials(ctx, env, profile.Profile, region)
		return provider, profile.Region, err
	}
	provider, err := envProfileCredentials(env, chain, region)
	return provider, profile.Region, err
}

func staticEnvCredentials(env map[string]string) aws.CredentialsProvider {
	return credentials.NewStaticCredentialsProvider(env["AWS_ACCESS_KEY_ID"], env["AWS_SECRET_ACCESS_KEY"], env["AWS_SESSION_TOKEN"])
}

// envWebIdentityCredentials exchanges the web identity token file that the environment names.
func envWebIdentityCredentials(env map[string]string, region, tokenFile string) (aws.CredentialsProvider, error) {
	roleARN := env["AWS_ROLE_ARN"]
	if roleARN == "" {
		return nil, fmt.Errorf("%w: AWS_WEB_IDENTITY_TOKEN_FILE is set without AWS_ROLE_ARN", errNoInvocationCredentials)
	}
	sessionName := env["AWS_ROLE_SESSION_NAME"]
	return stscreds.NewWebIdentityRoleProvider(invocationSTSClient(env, region, nil), roleARN, stscreds.IdentityTokenFile(tokenFile),
		func(o *stscreds.WebIdentityRoleOptions) { o.RoleSessionName = sessionName }), nil
}

// loadInvocationProfile reads the profile that the environment selects from the files that it names. A
// missing default profile is not an error and returns nil, as in the SDK; a missing named profile is.
func loadInvocationProfile(ctx context.Context, env map[string]string) (*config.SharedConfig, error) {
	name := cmp.Or(env["AWS_PROFILE"], defaultProfileName)
	configFile, credentialsFile := invocationSharedFiles(env)
	profile, err := config.LoadSharedConfigProfile(ctx, name, func(o *config.LoadSharedConfigOptions) {
		o.ConfigFiles = []string{configFile}
		o.CredentialsFiles = []string{credentialsFile}
	})
	var notExist config.SharedConfigProfileNotExistError
	switch {
	case err == nil:
		return &profile, nil
	case errors.As(err, &notExist) && env["AWS_PROFILE"] == "":
		return nil, nil
	default:
		return nil, err
	}
}

// invocationSharedFiles returns the shared config and credentials files of the environment, defaulting to
// the files in the home directory that the environment names.
func invocationSharedFiles(env map[string]string) (configFile, credentialsFile string) {
	configFile, credentialsFile = env["AWS_CONFIG_FILE"], env["AWS_SHARED_CREDENTIALS_FILE"]
	home := env["HOME"]
	if runtime.GOOS == "windows" {
		home = cmp.Or(env["USERPROFILE"], home)
	}
	if configFile == "" {
		configFile = config.DefaultSharedConfigFilename()
		if home != "" {
			configFile = filepath.Join(home, ".aws", "config")
		}
	}
	if credentialsFile == "" {
		credentialsFile = config.DefaultSharedCredentialsFilename()
		if home != "" {
			credentialsFile = filepath.Join(home, ".aws", "credentials")
		}
	}
	return configFile, credentialsFile
}

// profileChain lists a profile and its source profiles, ending with the profile that holds the source.
func profileChain(profile *config.SharedConfig) []*config.SharedConfig {
	chain := []*config.SharedConfig{profile}
	for current := profile; current.Source != nil; current = current.Source {
		chain = append(chain, current.Source)
	}
	return chain
}

// profileSourceNeedsEnvironment reports whether the SDK would take the profile's starting credentials from
// the environment: a credential_source, or no source at all so that container and instance credentials apply.
func profileSourceNeedsEnvironment(root *config.SharedConfig) bool {
	switch {
	case root.Credentials.HasKeys(), root.WebIdentityTokenFile != "", root.CredentialProcess != "", root.LoginSession != "":
		return false
	case root.SSOSessionName != "", root.SSORegion != "", root.SSOAccountID != "", root.SSOStartURL != "", root.SSORoleName != "":
		return false
	default:
		return true
	}
}

// sdkProfileCredentials lets the SDK resolve a profile whose source the shared files describe. The profile,
// files, and region are explicit, so the SDK does not consult the process environment to choose them.
func sdkProfileCredentials(ctx context.Context, env map[string]string, profile, region string) (aws.CredentialsProvider, error) {
	configFile, credentialsFile := invocationSharedFiles(env)
	options := []func(*config.LoadOptions) error{
		config.WithSharedConfigProfile(profile),
		config.WithSharedConfigFiles([]string{configFile}),
		config.WithSharedCredentialsFiles([]string{credentialsFile}),
		config.WithWebIdentityRoleCredentialOptions(func(o *stscreds.WebIdentityRoleOptions) {
			o.Client = invocationSTSClient(env, region, nil)
		}),
	}
	if region != "" {
		options = append(options, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrLoadAWSConfig, err)
	}
	return cfg.Credentials, nil
}

// envProfileCredentials resolves the source of a profile from the environment, then assumes each role of
// the chain from the source profile outward, as the SDK does.
func envProfileCredentials(env map[string]string, chain []*config.SharedConfig, region string) (aws.CredentialsProvider, error) {
	provider, err := profileSourceCredentials(env, chain[len(chain)-1])
	if err != nil {
		return nil, err
	}
	for i := len(chain) - 1; i >= 0; i-- {
		hop := chain[i]
		if hop.RoleARN == "" {
			continue
		}
		if hop.MFASerial != "" {
			return nil, config.AssumeRoleTokenProviderNotSetError{}
		}
		provider = aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(invocationSTSClient(env, region, provider), hop.RoleARN,
			func(o *stscreds.AssumeRoleOptions) { applyProfileRoleOptions(o, hop) }))
	}
	return provider, nil
}

func applyProfileRoleOptions(options *stscreds.AssumeRoleOptions, profile *config.SharedConfig) {
	options.RoleSessionName = profile.RoleSessionName
	if profile.RoleDurationSeconds != nil && *profile.RoleDurationSeconds >= minimumProfileRoleDuration {
		options.Duration = *profile.RoleDurationSeconds
	}
	if profile.ExternalID != "" {
		options.ExternalID = aws.String(profile.ExternalID)
	}
}

// profileSourceCredentials resolves a credential_source, or the container and instance fallback.
func profileSourceCredentials(env map[string]string, root *config.SharedConfig) (aws.CredentialsProvider, error) {
	switch root.CredentialSource {
	case "":
		return containerOrInstanceCredentials(env)
	case credentialSourceEnvironment:
		if env["AWS_ACCESS_KEY_ID"] == "" || env["AWS_SECRET_ACCESS_KEY"] == "" {
			return nil, fmt.Errorf("%w: credential_source %s", errNoInvocationCredentials, credentialSourceEnvironment)
		}
		return staticEnvCredentials(env), nil
	case credentialSourceContainer:
		provider, ok, err := containerCredentials(env)
		if err == nil && !ok {
			err = fmt.Errorf("%w: credential_source %s needs AWS_CONTAINER_CREDENTIALS_RELATIVE_URI or AWS_CONTAINER_CREDENTIALS_FULL_URI",
				errNoInvocationCredentials, credentialSourceContainer)
		}
		return provider, err
	case credentialSourceIMDS:
		return instanceCredentials(env), nil
	default:
		return nil, fmt.Errorf("%w: credential_source values must be %s, %s, or %s", errUtils.ErrInvalidAuthConfig,
			credentialSourceContainer, credentialSourceIMDS, credentialSourceEnvironment)
	}
}

func containerOrInstanceCredentials(env map[string]string) (aws.CredentialsProvider, error) {
	provider, ok, err := containerCredentials(env)
	if err != nil || ok {
		return provider, err
	}
	return instanceCredentials(env), nil
}

// containerCredentials returns the container credential endpoint that the environment names, if any. Like
// the SDK, it accepts a plain HTTP full URI only on a loopback or container metadata host.
func containerCredentials(env map[string]string) (aws.CredentialsProvider, bool, error) {
	endpoint := env["AWS_CONTAINER_CREDENTIALS_FULL_URI"]
	if relative := env["AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"]; relative != "" {
		endpoint = ecsContainerEndpoint + relative
	} else if endpoint == "" {
		return nil, false, nil
	} else if err := checkContainerEndpoint(endpoint); err != nil {
		return nil, false, err
	}
	token, tokenFile := env["AWS_CONTAINER_AUTHORIZATION_TOKEN"], env["AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"]
	provider := endpointcreds.New(endpoint, func(o *endpointcreds.Options) {
		o.AuthorizationToken = token
		if tokenFile != "" {
			o.AuthorizationTokenProvider = endpointcreds.TokenProviderFunc(func() (string, error) {
				contents, err := os.ReadFile(tokenFile)
				if err != nil {
					return "", fmt.Errorf("read the container authorization token file %s: %w", tokenFile, err)
				}
				return string(contents), nil
			})
		}
	})
	return aws.NewCredentialsCache(provider, func(o *aws.CredentialsCacheOptions) { o.ExpiryWindow = containerCredentialExpiry }), true, nil
}

func checkContainerEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%w: invalid container credentials URL: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: no host in the container credentials URL", errUtils.ErrInvalidAuthConfig)
	}
	if parsed.Scheme != "http" {
		return nil
	}
	addresses := []string{host}
	if net.ParseIP(host) == nil {
		if addresses, err = net.LookupHost(host); err != nil {
			return fmt.Errorf("%w: resolve container credentials host %q: %w", errUtils.ErrInvalidAuthConfig, host, err)
		}
	}
	for _, address := range addresses {
		if ip := net.ParseIP(address); ip == nil || !containerHostAllowed(ip) {
			return fmt.Errorf("%w: container credentials host %q is not a loopback, ECS, or EKS host", errUtils.ErrInvalidAuthConfig, host)
		}
	}
	return nil
}

func containerHostAllowed(ip net.IP) bool {
	return ip.IsLoopback() || slices.ContainsFunc(containerHosts, ip.Equal)
}

// instanceCredentials reads the instance role credentials. Every client setting is explicit, because the
// metadata client otherwise falls back to the process environment.
func instanceCredentials(env map[string]string) aws.CredentialsProvider {
	options := imds.Options{ClientEnableState: imds.ClientEnabled, Endpoint: env["AWS_EC2_METADATA_SERVICE_ENDPOINT"]}
	if strings.EqualFold(env["AWS_EC2_METADATA_DISABLED"], "true") {
		options.ClientEnableState = imds.ClientDisabled
	}
	if options.Endpoint == "" {
		options.Endpoint = imdsDefaultIPv4Endpoint
		if strings.EqualFold(env["AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE"], "IPv6") {
			options.Endpoint = imdsDefaultIPv6Endpoint
		}
	}
	if strings.EqualFold(env["AWS_EC2_METADATA_V1_DISABLED"], "true") {
		options.EnableFallback = aws.FalseTernary
	}
	return aws.NewCredentialsCache(ec2rolecreds.New(func(o *ec2rolecreds.Options) { o.Client = imds.New(options) }))
}

// invocationSTSClient builds an STS client from the environment's region and STS endpoint only.
func invocationSTSClient(env map[string]string, region string, credentials aws.CredentialsProvider) *sts.Client {
	options := sts.Options{Region: region, Credentials: credentials}
	if endpoint := cmp.Or(env["AWS_ENDPOINT_URL_STS"], env["AWS_ENDPOINT_URL"]); endpoint != "" {
		options.BaseEndpoint = aws.String(endpoint)
	}
	return sts.New(options)
}
