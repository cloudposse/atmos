package aws

import (
	"cmp"
	"context"
	"crypto/sha1" //nolint:gosec // The SDK and the AWS CLI name SSO token cache files after the SHA-1 of the cache key.
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/credentials/endpointcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
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
// that the shared files fully describe (keys, web identity, SSO, and login) are left to the SDK, with every
// input that selects them passed as an explicit option, including the SSO token cache of the invocation home.
// A credential_process runs from here too, because the SDK starts it with the environment of the process.

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
		provider, err := containerOrInstanceCredentials(env, nil)
		return provider, "", err
	}
	region = cmp.Or(region, profile.Region)
	chain := profileChain(profile)
	if root := chain[len(chain)-1]; !profileSourceNeedsEnvironment(root) {
		provider, err := sdkProfileCredentials(ctx, env, profile.Profile, region, root)
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
	selected := cmp.Or(env["AWS_PROFILE"], env["AWS_DEFAULT_PROFILE"])
	name := cmp.Or(selected, defaultProfileName)
	configFile, credentialsFile := invocationSharedFiles(env)
	profile, err := config.LoadSharedConfigProfile(ctx, name, func(o *config.LoadSharedConfigOptions) {
		o.ConfigFiles = []string{configFile}
		o.CredentialsFiles = []string{credentialsFile}
	})
	var notExist config.SharedConfigProfileNotExistError
	switch {
	case err == nil:
		return &profile, nil
	case errors.As(err, &notExist) && selected == "":
		return nil, nil
	default:
		return nil, err
	}
}

// invocationSharedFiles returns the shared config and credentials files of the environment, defaulting to
// the files in the home directory that the environment names.
func invocationSharedFiles(env map[string]string) (configFile, credentialsFile string) {
	configFile, credentialsFile = env["AWS_CONFIG_FILE"], env["AWS_SHARED_CREDENTIALS_FILE"]
	home := invocationHomeDir(env)
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

// invocationHomeDir returns the home directory that the environment names, or "".
func invocationHomeDir(env map[string]string) string {
	if runtime.GOOS == "windows" {
		return cmp.Or(env["USERPROFILE"], env["HOME"])
	}
	return env["HOME"]
}

// profileChain lists a profile and its source profiles, ending with the profile that holds the source.
func profileChain(profile *config.SharedConfig) []*config.SharedConfig {
	chain := []*config.SharedConfig{profile}
	for current := profile; current.Source != nil; current = current.Source {
		chain = append(chain, current.Source)
	}
	return chain
}

// profileSourceNeedsEnvironment reports whether the profile's starting credentials come from the invocation
// environment rather than from the shared files alone: a credential_source, a credential_process that runs
// with that environment, or no source at all so that container and instance credentials apply.
func profileSourceNeedsEnvironment(root *config.SharedConfig) bool {
	switch {
	case root.Credentials.HasKeys(), root.WebIdentityTokenFile != "", root.LoginSession != "":
		return false
	case root.SSOSessionName != "", root.SSORegion != "", root.SSOAccountID != "", root.SSOStartURL != "", root.SSORoleName != "":
		return false
	default:
		return true
	}
}

// profileSourceUsesSTSOnly reports whether the SDK builds only STS clients from the configuration that
// resolves a profile chain with this root, so that the STS endpoint can be the configuration's endpoint.
func profileSourceUsesSTSOnly(root *config.SharedConfig) bool {
	return root.SSOSessionName == "" && root.SSORegion == "" && root.SSOAccountID == "" && root.SSOStartURL == "" &&
		root.SSORoleName == "" && root.LoginSession == ""
}

// sdkProfileCredentials lets the SDK resolve a profile whose source the shared files describe. The profile,
// files, region, and SSO token cache are explicit, so the SDK does not consult the process environment to
// choose them. The root is the profile that holds the source of the chain.
func sdkProfileCredentials(ctx context.Context, env map[string]string, profile, region string, root *config.SharedConfig) (aws.CredentialsProvider, error) {
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
	options = append(options, ssoTokenCacheOptions(env, root)...)
	// The SDK builds the STS client of a role_arn and source_profile chain itself, so the invocation STS
	// endpoint is set as the endpoint of the loaded configuration. Only STS clients are built from it when
	// the chain starts from keys, a web identity, or a credential process; SSO and login chains also build
	// clients of other services, which this endpoint must not reach.
	if endpoint := invocationSTSEndpoint(env); endpoint != "" && profileSourceUsesSTSOnly(root) {
		options = append(options, config.WithBaseEndpoint(endpoint))
	}
	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrLoadAWSConfig, err)
	}
	return cfg.Credentials, nil
}

// ssoTokenCacheOptions points the SDK at the SSO token cache of the invocation home. The SDK derives the
// cache path from the home directory of the process, and a session profile and a legacy profile each have
// an option for it: a session's token provider is keyed by the session name, a legacy profile by its start
// URL, as in the AWS CLI. Anything else leaves the SDK's own resolution untouched.
func ssoTokenCacheOptions(env map[string]string, root *config.SharedConfig) []func(*config.LoadOptions) error {
	home := invocationHomeDir(env)
	switch {
	case home == "":
		return nil
	case root.SSOSessionName != "":
		path := ssoCachedTokenPath(home, root.SSOSessionName)
		return []func(*config.LoadOptions) error{config.WithSSOTokenProviderOptions(func(o *ssocreds.SSOTokenProviderOptions) {
			o.CachedTokenFilepath = path
		})}
	case root.SSOStartURL != "":
		path := ssoCachedTokenPath(home, root.SSOStartURL)
		return []func(*config.LoadOptions) error{config.WithSSOProviderOptions(func(o *ssocreds.Options) {
			o.CachedTokenFilepath = path
		})}
	default:
		return nil
	}
}

// ssoCachedTokenPath is the cache file of an SSO login: ~/.aws/sso/cache/<sha1-hex of the key>.json.
func ssoCachedTokenPath(home, key string) string {
	sum := sha1.Sum([]byte(key)) //nolint:gosec // The cache file name is defined by the AWS CLI, not chosen for security.
	return filepath.Join(home, ".aws", "sso", "cache", hex.EncodeToString(sum[:])+".json")
}

// envProfileCredentials resolves the source of a profile from the environment, then assumes each role of
// the chain from the source profile outward, as the SDK does.
func envProfileCredentials(env map[string]string, chain []*config.SharedConfig, region string) (aws.CredentialsProvider, error) {
	provider, err := profileSourceCredentials(env, chain)
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

// profileSourceCredentials resolves a credential_source, a credential_process, or the container and instance
// fallback, for the source at the end of the profile chain. The first profile of the chain is the one that
// the invocation selected, and it supplies the instance metadata settings, as in the SDK.
func profileSourceCredentials(env map[string]string, chain []*config.SharedConfig) (aws.CredentialsProvider, error) {
	root, selected := chain[len(chain)-1], chain[0]
	switch root.CredentialSource {
	case "":
		return unnamedSourceCredentials(env, root, selected)
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
		return instanceCredentials(env, selected), nil
	default:
		return nil, fmt.Errorf("%w: credential_source values must be %s, %s, or %s", errUtils.ErrInvalidAuthConfig,
			credentialSourceContainer, credentialSourceIMDS, credentialSourceEnvironment)
	}
}

// unnamedSourceCredentials resolves a profile source that names no credential_source: its credential_process
// when it has one, and the container and instance credentials otherwise.
func unnamedSourceCredentials(env map[string]string, root, selected *config.SharedConfig) (aws.CredentialsProvider, error) {
	if root.CredentialProcess != "" {
		return processCredentials(env, root.CredentialProcess), nil
	}
	return containerOrInstanceCredentials(env, selected)
}

func containerOrInstanceCredentials(env map[string]string, profile *config.SharedConfig) (aws.CredentialsProvider, error) {
	provider, ok, err := containerCredentials(env)
	if err != nil || ok {
		return provider, err
	}
	return instanceCredentials(env, profile), nil
}

// processCredentials runs a profile's credential_process with the invocation environment. The SDK starts it
// with the environment of the process, so the command is built here; the SDK's builder still supplies the
// shell, stderr, and stdin, and its provider the output parsing, timeout, and expiry.
func processCredentials(env map[string]string, command string) aws.CredentialsProvider {
	environ := make([]string, 0, len(env))
	for _, key := range slices.Sorted(maps.Keys(env)) {
		environ = append(environ, key+"="+env[key])
	}
	builder := processcreds.NewCommandBuilderFunc(func(ctx context.Context) (*exec.Cmd, error) {
		cmd, err := processcreds.DefaultNewCommandBuilder{Args: []string{command}}.NewCommand(ctx)
		if err != nil {
			return nil, err
		}
		cmd.Env = environ
		return cmd, nil
	})
	return aws.NewCredentialsCache(processcreds.NewProviderCommand(builder))
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
// metadata client otherwise falls back to the process environment. The invocation environment decides over
// the selected profile, which may be nil, as in the SDK.
func instanceCredentials(env map[string]string, profile *config.SharedConfig) aws.CredentialsProvider {
	options := imds.Options{ClientEnableState: imds.ClientEnabled, Endpoint: imdsEndpoint(env, profile)}
	if strings.EqualFold(env["AWS_EC2_METADATA_DISABLED"], "true") {
		options.ClientEnableState = imds.ClientDisabled
	}
	if imdsV1Disabled(env, profile) {
		options.EnableFallback = aws.FalseTernary
	}
	return aws.NewCredentialsCache(ec2rolecreds.New(func(o *ec2rolecreds.Options) { o.Client = imds.New(options) }))
}

// imdsEndpoint returns the metadata endpoint: an explicit endpoint wins over an endpoint mode, and the
// invocation environment wins over the profile at each step.
func imdsEndpoint(env map[string]string, profile *config.SharedConfig) string {
	profileEndpoint, profileMode := "", imds.EndpointModeStateUnset
	if profile != nil {
		profileEndpoint, profileMode = profile.EC2IMDSEndpoint, profile.EC2IMDSEndpointMode
	}
	if endpoint := cmp.Or(env["AWS_EC2_METADATA_SERVICE_ENDPOINT"], profileEndpoint); endpoint != "" {
		return endpoint
	}
	ipv6 := profileMode == imds.EndpointModeStateIPv6
	if mode := env["AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE"]; mode != "" {
		ipv6 = strings.EqualFold(mode, "IPv6")
	}
	if ipv6 {
		return imdsDefaultIPv6Endpoint
	}
	return imdsDefaultIPv4Endpoint
}

// imdsV1Disabled reports whether the fallback to IMDSv1 is restricted: a boolean in the invocation
// environment decides, and the selected profile's ec2_metadata_v1_disabled applies otherwise.
func imdsV1Disabled(env map[string]string, profile *config.SharedConfig) bool {
	if disabled, err := strconv.ParseBool(env["AWS_EC2_METADATA_V1_DISABLED"]); err == nil {
		return disabled
	}
	return profile != nil && profile.EC2IMDSv1Disabled != nil && *profile.EC2IMDSv1Disabled
}

// invocationSTSClient builds an STS client from the environment's region and STS endpoint only.
func invocationSTSClient(env map[string]string, region string, credentials aws.CredentialsProvider) *sts.Client {
	options := sts.Options{Region: region, Credentials: credentials}
	applyInvocationSTSEndpoint(env, &options)
	return sts.New(options)
}

// applyInvocationSTSEndpoint sets the STS endpoint that the invocation environment names, if any.
func applyInvocationSTSEndpoint(env map[string]string, options *sts.Options) {
	if endpoint := invocationSTSEndpoint(env); endpoint != "" {
		options.BaseEndpoint = aws.String(endpoint)
	}
}

// invocationSTSEndpoint returns the STS endpoint that the invocation environment names, or "".
func invocationSTSEndpoint(env map[string]string) string {
	return cmp.Or(env["AWS_ENDPOINT_URL_STS"], env["AWS_ENDPOINT_URL"])
}
