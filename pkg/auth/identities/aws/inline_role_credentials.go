package aws

import (
	"cmp"
	"context"
	"crypto/sha1" //nolint:gosec // The SDK and the AWS CLI name SSO token cache files after the SHA-1 of the cache key.
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
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
	"github.com/aws/aws-sdk-go-v2/credentials/logincreds"
	"github.com/aws/aws-sdk-go-v2/credentials/processcreds"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/signin"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// The pinned AWS SDK (config v1.33) reads its credential chain inputs with os.Getenv and offers no way to
// pass another environment, and its configuration loader also parses and validates the settings of the
// process. The base credentials of an inline role must come from the invocation environment alone, and
// changing the process environment would leak into concurrent invocations. So the chain is assembled here, in
// the SDK's order, from the invocation environment and the shared files that it names: static keys, a web
// identity token file, the shared profile and its source profiles, container credentials, then instance
// metadata. The SDK's config loader is never called; only its shared file parser and its credential providers
// are. A credential_process runs from here too, because the SDK starts it with the environment of the process.

const (
	credentialSourceEnvironment = "Environment"
	credentialSourceContainer   = "EcsContainer"
	credentialSourceIMDS        = "Ec2InstanceMetadata"

	defaultProfileName        = "default"
	ecsContainerEndpoint      = "http://169.254.170.2"
	imdsDefaultIPv4Endpoint   = "http://169.254.169.254"
	imdsDefaultIPv6Endpoint   = "http://[fd00:ec2::254]"
	containerCredentialExpiry = 5 * time.Minute
	// Bounds on a container credential request, so an endpoint that accepts a connection and never answers
	// cannot block the caller after its own deadline has passed.
	containerCredentialDialTimeout    = 3 * time.Second
	containerCredentialRequestTimeout = 10 * time.Second
	// The SDK passes duration_seconds on only when it exceeds 15 whole minutes; shorter values use the STS default.
	minimumProfileRoleDuration = 16 * time.Minute
)

// awsConfigDirName is the directory of the AWS shared files and caches below a home directory.
const awsConfigDirName = ".aws"

// containerHosts are the ECS and EKS container credential hosts that plain HTTP may reach.
var containerHosts = []net.IP{net.ParseIP("169.254.170.2"), net.ParseIP("169.254.170.23"), net.ParseIP("fd00:ec2::23")}

// errCredentialProcess hides broker output from authentication errors.
var errCredentialProcess = errors.New("credential_process failed to supply valid credentials")

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
	if key, secret := invocationEnvironmentKeys(env); key != "" && secret != "" {
		return staticEnvCredentials(env), "", nil
	}
	if tokenFile := env["AWS_WEB_IDENTITY_TOKEN_FILE"]; tokenFile != "" {
		return webIdentityWithProfileRegion(ctx, env, region, tokenFile)
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
	provider, err := envProfileCredentials(env, profileChain(profile), region)
	return provider, profile.Region, err
}

// webIdentityWithProfileRegion keeps web identity as the credential source and reads the selected profile
// only for its region, when the environment names none. It returns the region of the profile it read.
func webIdentityWithProfileRegion(ctx context.Context, env map[string]string, region, tokenFile string) (aws.CredentialsProvider, string, error) {
	profileRegion := ""
	if region == "" {
		profile, err := loadInvocationProfile(ctx, env)
		if err != nil {
			return nil, "", err
		}
		if profile != nil {
			profileRegion = profile.Region
		}
	}
	provider, err := envWebIdentityCredentials(env, cmp.Or(region, profileRegion), tokenFile)
	return provider, profileRegion, err
}

func staticEnvCredentials(env map[string]string) aws.CredentialsProvider {
	key, secret := invocationEnvironmentKeys(env)
	return credentials.NewStaticCredentialsProvider(key, secret, env["AWS_SESSION_TOKEN"])
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
		configFile = os.DevNull
		if home != "" {
			configFile = filepath.Join(home, awsConfigDirName, "config")
		}
	}
	if credentialsFile == "" {
		credentialsFile = os.DevNull
		if home != "" {
			credentialsFile = filepath.Join(home, awsConfigDirName, "credentials")
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

// ssoCachedTokenPath is the cache file of an SSO login: ~/.aws/sso/cache/<sha1-hex of the key>.json.
func ssoCachedTokenPath(home, key string) string {
	sum := sha1.Sum([]byte(key)) //nolint:gosec // The cache file name is defined by the AWS CLI, not chosen for security.
	return filepath.Join(home, awsConfigDirName, "sso", "cache", hex.EncodeToString(sum[:])+".json")
}

// envProfileCredentials resolves the source of a profile from the environment, then assumes each role of
// the chain from the source profile outward, as the SDK does.
func envProfileCredentials(env map[string]string, chain []*config.SharedConfig, region string) (aws.CredentialsProvider, error) {
	provider, err := profileSourceCredentials(env, chain, region)
	if err != nil {
		return nil, err
	}
	for i := len(chain) - 1; i >= 0; i-- {
		hop := chain[i]
		if hop.RoleARN == "" || (i == len(chain)-1 && webIdentityRoot(hop)) {
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

// webIdentityRoot reports whether the profile's role is the one its web identity token assumes, which the
// source credentials already did, so the role must not be assumed a second time.
func webIdentityRoot(profile *config.SharedConfig) bool {
	return !profile.Credentials.HasKeys() && profile.WebIdentityTokenFile != "" && profile.Source == nil
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
func profileSourceCredentials(env map[string]string, chain []*config.SharedConfig, region string) (aws.CredentialsProvider, error) {
	root, selected := chain[len(chain)-1], chain[0]
	switch {
	case root.Credentials.HasKeys():
		return credentials.StaticCredentialsProvider{Value: root.Credentials}, nil
	case root.CredentialSource != "":
		return namedSourceCredentials(env, root.CredentialSource, selected)
	case root.WebIdentityTokenFile != "":
		return stscreds.NewWebIdentityRoleProvider(invocationSTSClient(env, region, nil), root.RoleARN,
			stscreds.IdentityTokenFile(root.WebIdentityTokenFile), func(o *stscreds.WebIdentityRoleOptions) { o.RoleSessionName = root.RoleSessionName }), nil
	case usesSSO(root):
		return ssoCredentials(env, root)
	case root.LoginSession != "":
		return loginSessionCredentials(env, root, region)
	default:
		return unnamedSourceCredentials(env, root, selected)
	}
}

// usesSSO reports whether the profile names any SSO setting, in the session or the legacy form.
func usesSSO(root *config.SharedConfig) bool {
	return cmp.Or(root.SSOSessionName, root.SSOStartURL, root.SSORegion, root.SSOAccountID, root.SSORoleName) != ""
}

// ssoCredentials resolves an SSO profile from the cached login of the invocation home, in the session form
// (a token provider keyed by the session name) and the legacy form (keyed by the start URL), as in the AWS CLI.
func ssoCredentials(env map[string]string, root *config.SharedConfig) (aws.CredentialsProvider, error) {
	home := invocationHomeDir(env)
	if home == "" {
		return nil, fmt.Errorf("%w: SSO token cache requires an invocation home directory", errUtils.ErrInvalidAuthConfig)
	}
	startURL, ssoRegion, cacheKey := root.SSOStartURL, root.SSORegion, root.SSOStartURL
	if root.SSOSessionName != "" {
		if root.SSOSession == nil {
			return nil, fmt.Errorf("%w: SSO session %q is not defined", errUtils.ErrInvalidAuthConfig, root.SSOSessionName)
		}
		startURL, ssoRegion, cacheKey = cmp.Or(root.SSOSession.SSOStartURL, startURL), cmp.Or(root.SSOSession.SSORegion, ssoRegion), root.SSOSessionName
	}
	if startURL == "" || ssoRegion == "" || root.SSOAccountID == "" || root.SSORoleName == "" {
		return nil, fmt.Errorf("%w: an SSO profile needs a start URL, a region, an account ID, and a role name", errUtils.ErrInvalidAuthConfig)
	}
	path := ssoCachedTokenPath(home, cacheKey)
	portal := sso.Options{Region: ssoRegion}
	if endpoint := cmp.Or(env["AWS_ENDPOINT_URL_SSO"], env["AWS_ENDPOINT_URL"]); endpoint != "" {
		portal.BaseEndpoint = aws.String(endpoint)
	}
	return aws.NewCredentialsCache(ssocreds.New(sso.New(portal), root.SSOAccountID, root.SSORoleName, startURL, func(o *ssocreds.Options) {
		o.CachedTokenFilepath = path
		if root.SSOSessionName != "" {
			o.SSOTokenProvider = ssocreds.NewSSOTokenProvider(ssooidc.New(oidcOptions(env, ssoRegion)), path)
		}
	})), nil
}

func oidcOptions(env map[string]string, region string) ssooidc.Options {
	options := ssooidc.Options{Region: region}
	if endpoint := cmp.Or(env["AWS_ENDPOINT_URL_SSO_OIDC"], env["AWS_ENDPOINT_URL"]); endpoint != "" {
		options.BaseEndpoint = aws.String(endpoint)
	}
	return options
}

// namedSourceCredentials resolves the credential_source of a profile source: the invocation environment, a
// container endpoint, or the instance metadata service.
func namedSourceCredentials(env map[string]string, source string, selected *config.SharedConfig) (aws.CredentialsProvider, error) {
	switch source {
	case credentialSourceEnvironment:
		if key, secret := invocationEnvironmentKeys(env); key == "" || secret == "" {
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

// loginSessionCredentials resolves an `aws login` session from the invocation environment: the token cache is
// the directory that AWS_LOGIN_CACHE_DIRECTORY names, or the login cache below the invocation home.
func loginSessionCredentials(env map[string]string, root *config.SharedConfig, region string) (aws.CredentialsProvider, error) {
	dir := env["AWS_LOGIN_CACHE_DIRECTORY"]
	if home := invocationHomeDir(env); dir == "" && home != "" {
		dir = filepath.Join(home, awsConfigDirName, "login", "cache")
	}
	if dir == "" {
		return nil, fmt.Errorf("%w: login_session requires an invocation home or AWS_LOGIN_CACHE_DIRECTORY", errUtils.ErrInvalidAuthConfig)
	}
	path, err := logincreds.StandardCachedTokenFilepath(root.LoginSession, dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrLoadAWSConfig, err)
	}
	client := signin.New(signin.Options{Region: cmp.Or(region, root.Region)})
	login := logincreds.New(client, path)
	return aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		// The SDK provider dereferences a cache that holds a JSON null, so it is rejected here.
		if err := checkLoginCache(path); err != nil {
			return aws.Credentials{}, err
		}
		return login.Retrieve(ctx)
	})), nil
}

// checkLoginCache rejects a login token cache that decodes to nothing. A missing or unreadable cache is left
// for the SDK provider to report.
func checkLoginCache(path string) error {
	contents, readErr := os.ReadFile(path)
	var token *map[string]any
	if readErr == nil && json.Unmarshal(contents, &token) == nil && token == nil {
		return fmt.Errorf("%w: the login token cache %s is empty", errUtils.ErrAuthenticationFailed, path)
	}
	return nil
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
	provider := processcreds.NewProviderCommand(builder)
	return aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		value, err := provider.Retrieve(ctx)
		if err != nil {
			// SDK parse errors embed broker stdout, which can contain unregistered secrets.
			// Never retain that error in the wrapping chain.
			if ctx.Err() != nil {
				return aws.Credentials{}, ctx.Err()
			}
			return aws.Credentials{}, errCredentialProcess
		}
		return value, nil
	}))
}

// containerCredentials returns the container credential endpoint that the environment names, if any. Like
// the SDK, it accepts a plain HTTP full URI only on a loopback or container metadata host.
func containerCredentials(env map[string]string) (aws.CredentialsProvider, bool, error) {
	endpoint := env["AWS_CONTAINER_CREDENTIALS_FULL_URI"]
	if relative := env["AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"]; relative != "" {
		endpoint = ecsContainerEndpoint + relative
	} else if endpoint == "" {
		return nil, false, nil
	}
	client, err := checkContainerEndpoint(endpoint)
	if err != nil {
		return nil, false, err
	}
	token, tokenFile := env["AWS_CONTAINER_AUTHORIZATION_TOKEN"], env["AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE"]
	provider := endpointcreds.New(endpoint, func(o *endpointcreds.Options) {
		if client != nil {
			o.HTTPClient = client
		}
		o.AuthorizationToken = token
		if tokenFile != "" {
			o.AuthorizationTokenProvider = endpointcreds.TokenProviderFunc(func() (string, error) {
				contents, err := os.ReadFile(tokenFile) //nolint:gosec // G703: the path is AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE of the invocation, read as the SDK reads it.
				if err != nil {
					return "", fmt.Errorf("read the container authorization token file %s: %w", tokenFile, err)
				}
				return string(contents), nil
			})
		}
	})
	return aws.NewCredentialsCache(provider, func(o *aws.CredentialsCacheOptions) { o.ExpiryWindow = containerCredentialExpiry }), true, nil
}

// containerEndpointClient is the HTTP client for a plain-HTTP container credential endpoint. It dials only an
// address that passed the same allowlist as the endpoint URL, and it never uses a proxy, so a host name that
// resolves differently after the early check cannot redirect the request. The request as a whole is bounded by
// timeout, and establishing the connection by containerCredentialDialTimeout.
func containerEndpointClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: containerCredentialDialTimeout}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DisableKeepAlives:     true, // Providers are short-lived; retain no idle connections.
			DialContext:           validatedContainerDialer(net.DefaultResolver.LookupHost, dialer.DialContext),
			TLSHandshakeTimeout:   containerCredentialDialTimeout,
			ResponseHeaderTimeout: timeout,
		},
	}
}

// dialFunc opens a network connection.
type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// validatedContainerDialer resolves the host when it connects and dials one of the validated addresses itself,
// rather than handing the name to the network stack to resolve again.
func validatedContainerDialer(lookup func(ctx context.Context, host string) ([]string, error), dial dialFunc) dialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("%w: container credentials address %q: %w", errUtils.ErrInvalidAuthConfig, address, err)
		}
		addresses := []string{host}
		if net.ParseIP(host) == nil {
			if addresses, err = lookup(ctx, host); err != nil {
				return nil, fmt.Errorf("%w: resolve container credentials host %q: %w", errUtils.ErrInvalidAuthConfig, host, err)
			}
		}
		for _, candidate := range addresses {
			if ip := net.ParseIP(candidate); ip == nil || !containerHostAllowed(ip) {
				return nil, fmt.Errorf("%w: container credentials host %q is not a loopback, ECS, or EKS host", errUtils.ErrInvalidAuthConfig, host)
			}
		}
		var dialErr error
		for _, candidate := range addresses {
			conn, err := dial(ctx, network, net.JoinHostPort(candidate, port))
			if err == nil {
				return conn, nil
			}
			dialErr = err
		}
		return nil, dialErr
	}
}

// checkContainerEndpoint validates the parsed URL and selects the transport from its normalized scheme.
func checkContainerEndpoint(endpoint string) (*http.Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid container credentials URL: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	host := parsed.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%w: no host in the container credentials URL", errUtils.ErrInvalidAuthConfig)
	}
	if parsed.Scheme != "http" {
		return nil, nil
	}
	// A host name is resolved and checked by the validated dialer when the connection is made, under the
	// deadline of the request, so no resolver call without a deadline happens here.
	if ip := net.ParseIP(host); ip != nil && !containerHostAllowed(ip) {
		return nil, fmt.Errorf("%w: container credentials host %q is not a loopback, ECS, or EKS host", errUtils.ErrInvalidAuthConfig, host)
	}
	return containerEndpointClient(containerCredentialRequestTimeout), nil
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
