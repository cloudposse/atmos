package atmosbridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
)

// Upstream builds every AWS configuration with config.LoadDefaultConfig, which reads the plugin PROCESS
// environment. The invocation environment, which holds the selected Atmos identity (AWS_PROFILE and
// the shared credentials and config files), is invisible to it. The plugin serves concurrent
// invocations, so it must never copy the invocation into its own environment. Upstream does read three
// things from the venv it is handed: static keys in the venv environment, the region, and the HTTP
// client. AWS uses exactly those seams: it resolves the invocation's credentials with explicit SDK
// options, hands them to upstream as static keys, and routes endpoint overrides through the client.

const (
	awsSigningScopeParts = 5
	awsDefaultRegion     = "us-east-1"
	awsResolveTimeout    = 30 * time.Second
	// Placeholder for credentials that could not be resolved. Upstream then pins static
	// credentials and signs with them, and the transport answers every AWS request with the real reason.
	awsUnresolvedKey = "ATMOS_UNRESOLVED_AWS_CREDENTIALS"
	// Margin before expiry at which cached credentials are refreshed.
	awsExpiryMargin = time.Minute
	decimalBase     = 10
	int64Bits       = 64
	envAccessKeyID  = "AWS_ACCESS_KEY_ID"
	envSecretKey    = "AWS_SECRET_ACCESS_KEY"
	envSessionToken = "AWS_SESSION_TOKEN"
	envEndpointURL  = "AWS_ENDPOINT_URL"
)

// awsSelectors name the invocation variables that choose where credentials come from.
var awsSelectors = []string{"AWS_PROFILE", "AWS_DEFAULT_PROFILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE", "AWS_WEB_IDENTITY_TOKEN_FILE"}

// awsCache holds credentials resolved for an invocation selection until they expire. The key covers
// every selector and the shared files' size and time, so re-authentication invalidates it.
var awsCache = struct {
	sync.Mutex
	entries map[string]aws.Credentials
}{entries: map[string]aws.Credentials{}}

// AWS adapts v so that the AWS calls Terragrunt makes while evaluating a unit (get_aws_account_id,
// get_aws_caller_identity_arn, get_aws_caller_identity_user_id, S3 dependency state reads, and role
// assumption) use the invocation environment in v: the selected profile, shared credentials and config
// files, web identity token file, static keys, TG_IAM_ASSUME_ROLE, and AWS_ENDPOINT_URL[_<SERVICE>].
// The plugin process environment is not read or changed. Use it only for phases that may use the
// network; read-only discovery must not call it.
//
// Credentials that cannot be resolved never fail evaluation by themselves: a unit that does not call
// AWS is unaffected. A unit that does gets an error that names the cause instead of a misleading
// fallback to whatever credentials the plugin process happens to hold.
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func AWS(ctx context.Context, v *venv.Venv) *venv.Venv {
	env := maps.Clone(v.Env)
	if env == nil {
		env = map[string]string{}
	}
	transport := &awsTransport{next: v.HTTP.Transport, endpoints: awsEndpoints(env), credentials: aws.Credentials{
		AccessKeyID: env[envAccessKeyID], SecretAccessKey: env[envSecretKey], SessionToken: env[envSessionToken],
	}}
	role := roleOptions(env)
	switch {
	case role.RoleARN != "" && role.WebIdentityToken != "":
		// A command-level web identity role is assumed with an unsigned, invocation-scoped STS call, so no base
		// credentials are needed or resolved.
	case hasStaticAWSKeys(env):
	case selectsAWSCredentials(env):
		credentials, err := cachedCredentials(awsCacheKey(env), func() (aws.Credentials, error) { return resolveAWSCredentials(ctx, env, transport) })
		transport.apply(env, &credentials, err)
	default:
		// The invocation is authoritative: with no keys and no selection, upstream would fall back to the
		// SDK default chain, which reads the plugin process (its keys, container credentials, or instance role).
		transport.apply(env, nil, errNoInvocationCredentials)
	}
	if role.RoleARN != "" && transport.failure == nil {
		credentials, err := cachedCredentials(roleCacheKey(env, role), func() (aws.Credentials, error) { return assumeInvocationRole(ctx, v.WithEnv(env), transport, role) })
		transport.apply(env, &credentials, err)
	}
	if !transport.active() {
		return v.WithEnv(env)
	}
	return v.WithHTTP(vhttp.WithTransport(v.HTTP, transport)).WithEnv(env)
}

// hasStaticAWSKeys reports whether upstream already pins credentials from the environment.
func hasStaticAWSKeys(env map[string]string) bool {
	return env[envAccessKeyID] != "" && env[envSecretKey] != ""
}

func selectsAWSCredentials(env map[string]string) bool {
	return slices.ContainsFunc(awsSelectors, func(name string) bool { return env[name] != "" })
}

// roleOptions reads the CLI-level IAM role settings, which upstream applies to its own AWS calls.
func roleOptions(env map[string]string) iam.RoleOptions {
	pick := func(names ...string) string {
		for _, name := range names {
			if env[name] != "" {
				return env[name]
			}
		}
		return ""
	}
	duration, _ := strconv.ParseInt(pick("TG_IAM_ASSUME_ROLE_DURATION", "TERRAGRUNT_IAM_ASSUME_ROLE_DURATION"), decimalBase, int64Bits)
	return iam.RoleOptions{
		RoleARN:               pick("TG_IAM_ASSUME_ROLE", "TERRAGRUNT_IAM_ROLE"),
		AssumeRoleSessionName: pick("TG_IAM_ASSUME_ROLE_SESSION_NAME", "TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME"),
		WebIdentityToken:      pick("TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN", "TERRAGRUNT_IAM_WEB_IDENTITY_TOKEN", "TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"),
		AssumeRoleDuration:    duration,
	}
}

// errNoInvocationCredentials explains the failure of an AWS call from an invocation that supplies no credentials.
var errNoInvocationCredentials = errors.New("the invocation environment supplies no AWS credentials: set static keys, a profile, shared credential files, or a web identity token file; credentials of the plugin process are never used")

// assumeInvocationRole assumes the CLI-level role with the credentials already in v.
func assumeInvocationRole(ctx context.Context, v *venv.Venv, transport *awsTransport, role iam.RoleOptions) (aws.Credentials, error) {
	ctx, cancel := context.WithTimeout(ctx, awsResolveTimeout)
	defer cancel()
	if role.WebIdentityToken != "" {
		return assumeWebIdentityRole(ctx, v, transport, role)
	}
	// AssumeIamRole reads the HTTP client of the venv, so it must see the endpoint overrides.
	assumed, err := awshelper.AssumeIamRole(ctx, v.WithHTTP(vhttp.WithTransport(v.HTTP, transport)), role, "")
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("assuming the IAM role %s from the invocation environment: %w", role.RoleARN, err)
	}
	return aws.Credentials{
		AccessKeyID: aws.ToString(assumed.AccessKeyId), SecretAccessKey: aws.ToString(assumed.SecretAccessKey),
		SessionToken: aws.ToString(assumed.SessionToken), CanExpire: assumed.Expiration != nil, Expires: aws.ToTime(assumed.Expiration),
	}, nil
}

// apply installs credentials in env as static keys, or records why they are unavailable.
func (t *awsTransport) apply(env map[string]string, credentials *aws.Credentials, err error) {
	if err != nil {
		t.failure = err
		env[envAccessKeyID], env[envSecretKey], env[envSessionToken] = awsUnresolvedKey, awsUnresolvedKey, ""
		return
	}
	env[envAccessKeyID], env[envSecretKey], env[envSessionToken] = credentials.AccessKeyID, credentials.SecretAccessKey, credentials.SessionToken
	t.credentials = *credentials
}

// cachedCredentials loads credentials once per key, until they expire.
func cachedCredentials(key string, load func() (aws.Credentials, error)) (aws.Credentials, error) {
	awsCache.Lock()
	cached, found := awsCache.entries[key]
	awsCache.Unlock()
	if found && (!cached.CanExpire || time.Until(cached.Expires) > awsExpiryMargin) {
		return cached, nil
	}
	credentials, err := load()
	if err != nil {
		return aws.Credentials{}, err
	}
	awsCache.Lock()
	awsCache.entries[key] = credentials
	awsCache.Unlock()
	return credentials, nil
}

func awsCacheKey(env map[string]string) string {
	env = resolvedSharedFiles(env)
	hash := sha256.New()
	names := append(slices.Clone(awsSelectors), "AWS_ROLE_ARN", "AWS_ROLE_SESSION_NAME", "AWS_REGION", "AWS_DEFAULT_REGION")
	for name := range maps.Keys(env) {
		if strings.HasPrefix(name, envEndpointURL) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(hash, "%s=%s\n", name, env[name])
		if strings.HasSuffix(name, "_FILE") && env[name] != "" {
			if info, err := os.Stat(env[name]); err == nil {
				fmt.Fprintf(hash, "stat=%d:%d\n", info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// roleCacheKey identifies an assumed role session by every input of the assumption: the selection, the
// role settings, the credentials that assume it, and the web identity token, which may be the token itself
// or a file holding it. Secrets enter only as a digest, and a token file also by its size and time, so a
// rotated token is validated again instead of reusing the session of the previous one.
func roleCacheKey(env map[string]string, role iam.RoleOptions) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "selection=%s\nrole=%s\nsession=%s\nduration=%d\naccess=%s\n",
		awsCacheKey(env), role.RoleARN, role.AssumeRoleSessionName, role.AssumeRoleDuration, env[envAccessKeyID])
	for _, secret := range []string{env[envSecretKey], env[envSessionToken], role.WebIdentityToken} {
		digest := sha256.Sum256([]byte(secret))
		fmt.Fprintf(hash, "secret=%x\n", digest)
	}
	if role.WebIdentityToken != "" {
		if info, err := os.Stat(role.WebIdentityToken); err == nil {
			fmt.Fprintf(hash, "token=%d:%d\n", info.Size(), info.ModTime().UnixNano())
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// resolveAWSCredentials loads the invocation's selection with explicit SDK options. An explicit profile
// outranks the static keys of the plugin process, which the SDK would otherwise prefer.
func resolveAWSCredentials(ctx context.Context, env map[string]string, transport *awsTransport) (aws.Credentials, error) {
	ctx, cancel := context.WithTimeout(ctx, awsResolveTimeout)
	defer cancel()
	// The SDK reads the shared files again when it resolves, so validation and resolution share one private
	// copy of them: a file that changes in between cannot select a source that was never validated. The SDK
	// has no option that accepts an already loaded profile.
	env, cleanup, err := snapshotSharedFiles(env)
	if err != nil {
		return aws.Credentials{}, err
	}
	defer cleanup()
	if err := rejectAmbientCredentialSources(ctx, env); err != nil {
		return aws.Credentials{}, err
	}
	afterProfileValidated()
	cfg, err := invocationConfig(ctx, env, transport)
	if err != nil {
		return aws.Credentials{}, err
	}
	credentials, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("resolving AWS credentials from the invocation environment (profile %q): %w", profileName(env), err)
	}
	return credentials, nil
}

// invocationConfig builds the SDK configuration of the invocation. A web identity role calls STS from the
// invocation's values alone, so no process credential chain is loaded for it; every other selection loads
// the invocation's shared files with explicit options.
func invocationConfig(ctx context.Context, env map[string]string, transport *awsTransport) (aws.Config, error) {
	if selectsWebIdentityRole(env) {
		return webIdentityConfig(env, transport), nil
	}
	cfg, err := config.LoadDefaultConfig(ctx, loadOptions(env, transport)...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("loading the AWS configuration selected by the invocation environment: %w", err)
	}
	return cfg, nil
}

// webIdentityConfig assumes the invocation's web identity role. AssumeRoleWithWebIdentity is unsigned, so the
// STS client needs no credentials; it keeps the invocation's region, the endpoint routing and path prefix of the
// transport, and the resolution timeout.
func webIdentityConfig(env map[string]string, transport *awsTransport) aws.Config {
	cfg := aws.Config{
		Region:      awsRegion(env),
		Credentials: aws.AnonymousCredentials{},
		HTTPClient:  &http.Client{Transport: transport, Timeout: awsResolveTimeout},
		APIOptions:  []func(*middleware.Stack) error{transport.prefixBeforeSigning},
	}
	provider := stscreds.NewWebIdentityRoleProvider(sts.NewFromConfig(cfg), env["AWS_ROLE_ARN"], stscreds.IdentityTokenFile(env["AWS_WEB_IDENTITY_TOKEN_FILE"]), func(o *stscreds.WebIdentityRoleOptions) {
		o.RoleSessionName = env["AWS_ROLE_SESSION_NAME"]
	})
	cfg.Credentials = aws.NewCredentialsCache(provider)
	return cfg
}

// awsRegion is the invocation's region, or the default one.
func awsRegion(env map[string]string) string {
	return cmpFirst(env["AWS_REGION"], env["AWS_DEFAULT_REGION"], awsDefaultRegion)
}

func profileName(env map[string]string) string {
	if name := env["AWS_PROFILE"]; name != "" {
		return name
	}
	if name := env["AWS_DEFAULT_PROFILE"]; name != "" {
		return name
	}
	return "default"
}

func loadOptions(env map[string]string, transport *awsTransport) []func(*config.LoadOptions) error {
	env = resolvedSharedFiles(env)
	// The transport carries the endpoint overrides for the STS calls that role and web identity profiles make.
	options := []func(*config.LoadOptions) error{
		config.WithRegion(awsRegion(env)),
		// Clear process/shared endpoint selections before STS resolves or signs its request.
		// The invocation transport applies its own endpoint and prefix to the default AWS host.
		config.WithServiceOptions(func(_ string, options any) {
			if stsOptions, ok := options.(*sts.Options); ok {
				stsOptions.BaseEndpoint = nil
			}
		}),
		config.WithHTTPClient(&http.Client{Transport: transport, Timeout: awsResolveTimeout}),
		// A role chain signs its STS calls with source credentials the transport does not hold, so the
		// endpoint path prefix is applied before the SDK signs them instead of re-signing in the transport.
		config.WithAPIOptions([]func(*middleware.Stack) error{transport.prefixBeforeSigning}),
	}
	if strings.EqualFold(env["AWS_EC2_METADATA_DISABLED"], "true") {
		options = append(options, config.WithEC2IMDSClientEnableState(imds.ClientDisabled))
	}
	// LoadDefaultConfig also reads the plugin process environment, and its static keys, web identity token
	// file, profile, and shared file locations outrank the shared files unless a profile and the files are
	// set explicitly. So both are: the invocation's selection, or the SDK defaults it would use when the
	// invocation leaves one out. Credentials then come only from the invocation's shared files.
	return append(
		options,
		config.WithSharedConfigProfile(profileName(env)),
		config.WithSharedCredentialsFiles([]string{env["AWS_SHARED_CREDENTIALS_FILE"]}),
		config.WithSharedConfigFiles([]string{env["AWS_CONFIG_FILE"]}),
	)
}

// selectsWebIdentityRole reports whether the invocation selects a web identity role instead of a profile.
func selectsWebIdentityRole(env map[string]string) bool {
	return env["AWS_PROFILE"] == "" && env["AWS_DEFAULT_PROFILE"] == "" && env["AWS_WEB_IDENTITY_TOKEN_FILE"] != "" && env["AWS_ROLE_ARN"] != ""
}

func cmpFirst(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// awsTransport sends AWS-bound requests to the endpoints the invocation overrides, and answers them with
// the resolution failure when the invocation's credentials could not be resolved. Requests to other
// hosts pass through untouched. The request keeps its original Host header; a configured path prefix
// is added to the service path and the request is signed again for that final path.
type awsTransport struct {
	next        http.RoundTripper
	endpoints   map[string]*url.URL
	failure     error
	credentials aws.Credentials
}

// active reports whether the transport has anything to do.
func (t *awsTransport) active() bool { return t.failure != nil || len(t.endpoints) > 0 }

// ErrAWSCredentials marks a request that could not be signed with the invocation's credentials.
var ErrAWSCredentials = errors.New("the AWS credentials of the invocation environment could not be resolved")

// unresolvedError is the answer to an AWS request that has no usable credentials. The SDK must not retry it.
type unresolvedError struct{ cause error }

//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func (e unresolvedError) Error() string { return ErrAWSCredentials.Error() + ": " + e.cause.Error() }

//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func (e unresolvedError) Unwrap() []error { return []error{ErrAWSCredentials, e.cause} }

//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func (e unresolvedError) RetryableError() bool { return false }

//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func (t *awsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	if !isAWSHost(req.URL.Hostname()) {
		return next.RoundTrip(req)
	}
	if t.failure != nil {
		return nil, unresolvedError{cause: t.failure}
	}
	target := t.endpointFor(req.URL.Hostname())
	if target == nil {
		return next.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Host = req.URL.Host
	clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
	if hasPathPrefix(target) && req.Context().Value(prefixedKey{}) == nil {
		clone.URL.Path, clone.URL.RawPath = prefixedPath(target, req.URL.Path), ""
		if err := t.signPrefixedRequest(clone); err != nil {
			return nil, err
		}
	}
	return next.RoundTrip(clone)
}

// prefixedKey marks a request whose endpoint path prefix was applied before signing.
type prefixedKey struct{}

// prefixBeforeSigning adds a step, run just before the SDK signs a request, that applies the endpoint path
// prefix of an overridden AWS host. The signature then covers the final path whatever credentials sign it,
// and the transport only changes the host.
func (t *awsTransport) prefixBeforeSigning(stack *middleware.Stack) error {
	step := middleware.FinalizeMiddlewareFunc("AtmosEndpointPathPrefix", func(ctx context.Context, in middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
		if req, ok := in.Request.(*smithyhttp.Request); ok && isAWSHost(req.URL.Hostname()) {
			if target := t.endpointFor(req.URL.Hostname()); hasPathPrefix(target) {
				req.URL.Path, req.URL.RawPath = prefixedPath(target, req.URL.Path), ""
				ctx = context.WithValue(ctx, prefixedKey{}, true)
			}
		}
		return next.HandleFinalize(ctx, in)
	})
	return stack.Finalize.Insert(step, "Signing", middleware.Before)
}

func hasPathPrefix(target *url.URL) bool {
	return target != nil && target.Path != "" && target.Path != "/"
}

func prefixedPath(target *url.URL, path string) string {
	return strings.TrimRight(target.Path, "/") + "/" + strings.TrimLeft(path, "/")
}

// signPrefixedRequest updates a SigV4 signature after the path changes. The upstream SDK
// signs before RoundTrip, so retaining its signature would authenticate the old path.
func (t *awsTransport) signPrefixedRequest(req *http.Request) error {
	authorization := req.Header.Get("Authorization")
	if authorization == "" {
		return nil
	}
	parts := strings.SplitN(strings.TrimPrefix(authorization, "AWS4-HMAC-SHA256 Credential="), ",", 2)
	if len(parts) != 2 || parts[0] == authorization {
		return fmt.Errorf("%w: cannot re-sign AWS request with endpoint path prefix", ErrAWSCredentials)
	}
	scope := strings.Split(parts[0], "/")
	if len(scope) != awsSigningScopeParts || scope[awsSigningScopeParts-1] != "aws4_request" || scope[0] != t.credentials.AccessKeyID || t.credentials.SecretAccessKey == "" {
		return fmt.Errorf("%w: AWS signing scope or credentials changed", ErrAWSCredentials)
	}
	stamp, err := time.Parse("20060102T150405Z", req.Header.Get("X-Amz-Date"))
	if err != nil {
		return fmt.Errorf("%w: AWS signing time is missing: %w", ErrAWSCredentials, err)
	}
	payloadHash, err := requestPayloadHash(req)
	if err != nil {
		return err
	}
	req.Header.Del("Authorization")
	return v4.NewSigner().SignHTTP(req.Context(), t.credentials, req, payloadHash, scope[3], scope[2], stamp, func(options *v4.SignerOptions) {
		// S3 signs the escaped wire path without escaping percent signs a second time.
		options.DisableURIPathEscaping = scope[3] == "s3"
	})
}

// requestPayloadHash preserves an existing hash or hashes and restores the request body.
func requestPayloadHash(req *http.Request) (string, error) {
	if hash := req.Header.Get("X-Amz-Content-Sha256"); hash != "" {
		return hash, nil
	}
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return "", err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func isAWSHost(host string) bool {
	return strings.HasSuffix(host, ".amazonaws.com") || strings.HasSuffix(host, ".amazonaws.com.cn")
}

// endpointFor picks the service-specific override for host, falling back to the global one.
func (t *awsTransport) endpointFor(host string) *url.URL {
	labels := strings.Split(host, ".")
	candidates := []string{labels[0]}
	if slices.ContainsFunc(labels, func(label string) bool { return label == "s3" || strings.HasPrefix(label, "s3-") }) {
		candidates = []string{"s3"}
	}
	for _, service := range candidates {
		if target := t.endpoints[service]; target != nil {
			return target
		}
	}
	return t.endpoints[""]
}

// awsEndpoints reads AWS_ENDPOINT_URL and AWS_ENDPOINT_URL_<SERVICE>. The global entry has the empty key.
// A service key is lower case without separators, matching the first label of the service's host name.
func awsEndpoints(env map[string]string) map[string]*url.URL {
	endpoints := map[string]*url.URL{}
	for name, value := range env {
		if value == "" || !strings.HasPrefix(name, envEndpointURL) {
			continue
		}
		service := strings.TrimPrefix(strings.TrimPrefix(name, envEndpointURL), "_")
		if name != envEndpointURL && service == "" {
			continue
		}
		target, err := url.Parse(value)
		if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
			continue
		}
		endpoints[strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(service))] = target
	}
	return endpoints
}

// resolvedSharedFiles binds cache fingerprints to the same explicit paths the SDK loads.
func resolvedSharedFiles(env map[string]string) map[string]string {
	resolved := maps.Clone(env)
	if resolved == nil {
		resolved = map[string]string{}
	}
	resolved["AWS_SHARED_CREDENTIALS_FILE"] = cmpFirst(env["AWS_SHARED_CREDENTIALS_FILE"], config.DefaultSharedCredentialsFilename())
	resolved["AWS_CONFIG_FILE"] = cmpFirst(env["AWS_CONFIG_FILE"], config.DefaultSharedConfigFilename())
	return resolved
}

// rejectAmbientCredentialSources rejects profile sources the SDK resolves from process state.
func rejectAmbientCredentialSources(ctx context.Context, env map[string]string) error {
	if selectsWebIdentityRole(env) {
		return nil
	}
	files := resolvedSharedFiles(env)
	selected, err := config.LoadSharedConfigProfile(ctx, profileName(env), func(options *config.LoadSharedConfigOptions) {
		options.CredentialsFiles = []string{files["AWS_SHARED_CREDENTIALS_FILE"]}
		options.ConfigFiles = []string{files["AWS_CONFIG_FILE"]}
	})
	if err != nil {
		return err
	}
	var terminal *config.SharedConfig
	for profile := &selected; profile != nil; profile = profile.Source {
		if profile.CredentialSource != "" {
			return fmt.Errorf("%w: profile credential_source cannot be isolated from the plugin process; use invocation static credentials or a file-backed source_profile", ErrAWSCredentials)
		}
		// The SDK starts the helper with the plugin process environment and offers no option for another one.
		if profile.CredentialProcess != "" {
			return fmt.Errorf("%w: profile credential_process runs with the plugin process environment, not the invocation's; use invocation static credentials, SSO settings, or a web identity token file instead", ErrAWSCredentials)
		}
		terminal = profile
	}
	if !hasFileBackedCredentials(terminal) {
		return fmt.Errorf("%w: profile %q has no credential source in the invocation's shared files, and the SDK would fall back to the plugin process; add static keys, SSO settings, or a web identity token file to the profile", ErrAWSCredentials, profileName(env))
	}
	return nil
}

// hasFileBackedCredentials reports whether the last profile of a chain names a source the SDK resolves from
// the shared files: static keys, SSO settings, or a role from a web identity token file. A credential process
// is not one of them: the SDK would run it with the plugin process environment.
func hasFileBackedCredentials(profile *config.SharedConfig) bool {
	return profile != nil && (profile.Credentials.HasKeys() ||
		profile.SSOSessionName != "" || profile.SSOStartURL != "" || profile.SSOAccountID != "" ||
		(profile.WebIdentityTokenFile != "" && profile.RoleARN != ""))
}

const (
	// minAssumeRoleDuration and maxAssumeRoleDuration are the session durations in seconds that STS accepts.
	minAssumeRoleDuration = 900
	maxAssumeRoleDuration = 43200
)

// webIdentityDuration returns the session duration for the web identity assumption: the default when none is
// set, otherwise the configured seconds when STS would accept them. The range check also bounds the value
// before it narrows to the 32 bits of the STS request.
func webIdentityDuration(seconds int64) (int32, error) {
	if seconds <= 0 {
		return iam.DefaultAssumeRoleDuration, nil
	}
	if seconds < minAssumeRoleDuration || seconds > maxAssumeRoleDuration {
		return 0, fmt.Errorf("%w: the session duration of %d seconds is outside the %d to %d seconds STS accepts",
			ErrAWSCredentials, seconds, minAssumeRoleDuration, maxAssumeRoleDuration)
	}
	return int32(seconds), nil
}

// assumeWebIdentityRole assumes the CLI-level role with its web identity token. The token is the credential,
// so the STS client is unsigned and built from the invocation's values alone; upstream would load the plugin
// process environment for it.
func assumeWebIdentityRole(ctx context.Context, v *venv.Venv, transport *awsTransport, role iam.RoleOptions) (aws.Credentials, error) {
	token, err := awshelper.TokenFetcher{FS: v.FS, Token: role.WebIdentityToken}.FetchToken(ctx)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("reading the web identity token of the IAM role %s: %w", role.RoleARN, err)
	}
	name := cmpFirst(role.AssumeRoleSessionName, iam.GetDefaultAssumeRoleSessionName())
	duration, err := webIdentityDuration(role.AssumeRoleDuration)
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("assuming the IAM role %s with its web identity token: %w", role.RoleARN, err)
	}
	client := sts.NewFromConfig(aws.Config{
		Region:      awsRegion(v.Env),
		Credentials: aws.AnonymousCredentials{},
		HTTPClient:  &http.Client{Transport: transport, Timeout: awsResolveTimeout},
		APIOptions:  []func(*middleware.Stack) error{transport.prefixBeforeSigning},
	})
	result, err := client.AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{
		RoleArn: aws.String(role.RoleARN), RoleSessionName: aws.String(name), WebIdentityToken: aws.String(string(token)),
		DurationSeconds: aws.Int32(duration),
	})
	if err != nil {
		return aws.Credentials{}, fmt.Errorf("assuming the IAM role %s with its web identity token: %w", role.RoleARN, err)
	}
	if result.Credentials == nil {
		return aws.Credentials{}, fmt.Errorf("assuming the IAM role %s with its web identity token: %w", role.RoleARN, ErrAWSCredentials)
	}
	return aws.Credentials{
		AccessKeyID: aws.ToString(result.Credentials.AccessKeyId), SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken: aws.ToString(result.Credentials.SessionToken), CanExpire: result.Credentials.Expiration != nil, Expires: aws.ToTime(result.Credentials.Expiration),
	}, nil
}

// snapshotSharedFiles copies the shared credentials and config files into a private directory and returns env
// pointing at the copies. A file that does not exist stays missing. Web identity roles read no shared file.
func snapshotSharedFiles(env map[string]string) (map[string]string, func(), error) {
	if selectsWebIdentityRole(env) {
		return env, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "atmos-aws-snapshot-")
	if err != nil {
		return nil, nil, fmt.Errorf("%w: snapshotting the shared AWS files: %w", ErrAWSCredentials, err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	files, snapshot := resolvedSharedFiles(env), maps.Clone(env)
	for _, name := range []string{"AWS_SHARED_CREDENTIALS_FILE", "AWS_CONFIG_FILE"} {
		copied := filepath.Join(dir, strings.ToLower(name))
		snapshot[name] = copied
		content, err := os.ReadFile(files[name])
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil {
			err = os.WriteFile(copied, content, 0o600)
		}
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("%w: snapshotting the shared AWS file %s: %w", ErrAWSCredentials, files[name], err)
		}
	}
	return snapshot, cleanup, nil
}

// afterProfileValidated runs between the validation of a profile snapshot and its resolution. Tests use it to
// change the shared files in that window.
var afterProfileValidated = func() {}
