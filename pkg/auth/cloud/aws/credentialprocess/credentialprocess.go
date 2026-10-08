// Package credentialprocess produces AWS process-credential documents for Atmos identities.
//
// It backs `atmos aws credential-process` and `atmos auth env --format=credential-process`, so
// both entry points emit byte-identical output. The document follows the AWS SDK
// `credential_process` contract:
// https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html.
package credentialprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DefaultMinValidity is the default remaining lifetime cached credentials must have to be
	// reused without authenticating again. The AWS CLI runs credential_process once per invocation,
	// so reusing credentials that are still valid for a while keeps every `aws` command fast.
	// It is the same validity buffer the auth manager applies to its credential chain.
	DefaultMinValidity = types.DefaultMinCredentialValidity

	// MinValidityFlagName is the name of the flag that sets the minimum remaining credential lifetime.
	MinValidityFlagName = "min-validity"

	// MinValidityEnvVar is the environment variable equivalent of the --min-validity flag.
	MinValidityEnvVar = "ATMOS_AWS_CREDENTIAL_PROCESS_MIN_VALIDITY"

	// Remediation shown when no concrete identity can be resolved.
	identityHint = "Pass `--identity=<name>` or set `ATMOS_IDENTITY`; run `atmos auth list` to see the available identities."
)

// fileMode is the permission for files that contain credentials.
const fileMode os.FileMode = 0o600

type options struct {
	minValidity time.Duration
	now         func() time.Time
}

// Option configures Produce.
type Option func(*options)

// WithMinValidity sets how long cached credentials must remain valid to be reused. A zero or
// negative value reuses any credentials that are not yet expired.
func WithMinValidity(d time.Duration) Option {
	return func(o *options) {
		o.minValidity = d
	}
}

// WithClock overrides the clock used to evaluate credential expiration (used by tests).
func WithClock(now func() time.Time) Option {
	return func(o *options) {
		if now != nil {
			o.now = now
		}
	}
}

// Produce returns the AWS process-credential JSON document for the identity.
//
// Cached temporary credentials are used when they expire after now plus the minimum validity,
// which requires no network call. Otherwise the identity is authenticated the same way
// `atmos auth login` would, and standalone identities that keep their own cache refresh it when it
// does not satisfy the minimum validity. When the source still issues credentials that expire sooner
// than requested (for example a helper that only mints 20 minute credentials), those credentials are
// returned as-is: the command neither loops nor fails. Cached credentials without an expiration are
// never printed directly: for an IAM user they are the long-lived access keys, and authenticating exchanges them for a
// session (honoring MFA) instead of handing the keys to the caller. Auto-triggered integrations (for example
// kubeconfig or ECR login) are skipped because this runs as a non-interactive helper.
func Produce(ctx context.Context, mgr types.AuthManager, identityName string, opts ...Option) ([]byte, error) {
	defer perf.Track(nil, "credentialprocess.Produce")()

	o := options{minValidity: DefaultMinValidity, now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}

	ctx = auth.ContextWithSkipIntegrations(ctx)
	// Identities that cache their own credentials (aws/user sessions, aws/credential-process helper
	// output) must honor the requested minimum validity too, not only the cached check below.
	ctx = types.WithMinCredentialValidity(ctx, o.minValidity)

	creds, err := resolveCredentials(ctx, mgr, identityName, &o)
	if err != nil {
		return nil, err
	}

	return awsCloud.MarshalProcessCredentials(creds)
}

// Render returns the document as written to stdout or a file: the JSON followed by a newline.
// Every entry point uses it so their output is byte-identical.
func Render(doc []byte) string {
	defer perf.Track(nil, "credentialprocess.Render")()

	return string(doc) + "\n"
}

// FormatMinValidity formats a minimum validity for display, for example 15m rather than 15m0s.
func FormatMinValidity(d time.Duration) string {
	defer perf.Track(nil, "credentialprocess.FormatMinValidity")()

	text := d.String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}

// ParseMinValidity parses a --min-validity value. Negative durations are rejected.
func ParseMinValidity(value string) (time.Duration, error) {
	defer perf.Track(nil, "credentialprocess.ParseMinValidity")()

	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w --%s=%q: %w", errUtils.ErrInvalidFlagValue, MinValidityFlagName, value, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("%w --%s=%q: must not be negative", errUtils.ErrInvalidFlagValue, MinValidityFlagName, value)
	}
	return d, nil
}

// WriteFile writes the rendered document to path, replacing any existing content. The file
// holds secrets, so it is never readable by anyone but the owner, not even briefly: the content
// goes to a temporary file created with mode 0600 in the same directory, which then atomically
// replaces the target. An existing file with looser permissions therefore never holds the secrets.
func WriteFile(path string, doc []byte) (err error) {
	defer perf.Track(nil, "credentialprocess.WriteFile")()

	writeErr := func(cause error) error {
		return errUtils.Build(errUtils.ErrWriteFile).
			WithCause(cause).
			WithContext("path", path).
			Err()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return writeErr(err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// The temp file is gone after a successful rename; this only cleans up failures.
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	// CreateTemp already creates the file with mode 0600. Chmod keeps that explicit and independent of
	// the implementation (it is a no-op for the permission bits that Windows does not model).
	if err = tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		return writeErr(err)
	}
	if _, err = tmp.WriteString(Render(doc)); err != nil {
		_ = tmp.Close()
		return writeErr(err)
	}
	if err = tmp.Close(); err != nil {
		return writeErr(err)
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return writeErr(err)
	}
	return nil
}

// ResolveIdentity resolves the identity to produce credentials for. A concrete name is returned
// as-is and an empty name falls back to the configured default identity.
//
// It never opens the interactive identity selector: the AWS CLI captures this command's stderr, so
// a prompt would be invisible and the `aws` command would hang forever. Both the disabled sentinel
// (--identity=false) and the select sentinel (--identity without a value) are therefore rejected,
// and a missing default fails fast even when stdin is a terminal.
func ResolveIdentity(mgr types.AuthManager, identityName string) (string, error) {
	defer perf.Track(nil, "credentialprocess.ResolveIdentity")()

	// Callers differ in whether they have already mapped false-like values ("false", "0", "off") to the
	// disabled sentinel, so normalize here to treat every entry point (flag, ATMOS_IDENTITY) the same.
	switch cfg.NormalizeIdentityValue(identityName) {
	case cfg.IdentityFlagDisabledValue:
		return "", errUtils.Build(errUtils.ErrCredentialProcessIdentityRequired).
			WithExplanation("Authentication cannot be disabled (`--identity=false`) because this command exists to print the credentials of an identity.").
			WithHint(identityHint).
			Err()
	case cfg.IdentityFlagSelectValue:
		return "", errUtils.Build(errUtils.ErrCredentialProcessIdentityRequired).
			WithExplanation("The interactive identity selector is not available because the AWS CLI captures this command's output, so a prompt would never be shown.").
			WithHint(identityHint).
			Err()
	case "":
		return defaultIdentity(mgr)
	}
	return identityName, nil
}

// defaultIdentity returns the single identity marked as default in the auth configuration. Unlike
// AuthManager.GetDefaultIdentity it never prompts.
func defaultIdentity(mgr types.AuthManager) (string, error) {
	identities := mgr.GetIdentities()
	var defaults []string
	for name := range identities {
		if identities[name].Default {
			defaults = append(defaults, name)
		}
	}
	sort.Strings(defaults)

	switch len(defaults) {
	case 0:
		return "", errUtils.Build(errUtils.ErrNoDefaultIdentity).
			WithHint(identityHint).
			Err()
	case 1:
		return defaults[0], nil
	default:
		return "", errUtils.Build(fmt.Errorf("%w: %s", errUtils.ErrMultipleDefaultIdentities, strings.Join(defaults, ", "))).
			WithHint(identityHint).
			Err()
	}
}

// resolveCredentials returns reusable cached AWS credentials, or authenticates for fresh ones.
func resolveCredentials(ctx context.Context, mgr types.AuthManager, identityName string, o *options) (*types.AWSCredentials, error) {
	cached, err := reusableCachedCredentials(ctx, mgr, identityName, o)
	if err != nil || cached != nil {
		return cached, err
	}

	awsCreds, err := authenticateAWS(ctx, mgr, identityName)
	if err != nil {
		return nil, err
	}
	if o.minValidity > 0 && awsCreds.Expiration != "" && !validLongEnough(awsCreds, o) {
		log.Debug("Credentials from the source expire sooner than the requested minimum validity, returning them anyway",
			"identity", identityName, "min_validity", o.minValidity, "expiration", awsCreds.Expiration)
	}
	return awsCreds, nil
}

// reusableCachedCredentials returns the cached AWS credentials when they can be printed without
// authenticating, and nil when authentication is needed.
func reusableCachedCredentials(ctx context.Context, mgr types.AuthManager, identityName string, o *options) (*types.AWSCredentials, error) {
	cached, err := mgr.GetCachedCredentials(ctx, identityName)
	if err != nil {
		log.Debug("No valid cached credentials, authenticating", "identity", identityName, "error", err)
		return nil, nil
	}
	if cached == nil || cached.Credentials == nil {
		return nil, nil
	}

	awsCreds, err := requireAWS(cached.Credentials, identityName)
	if err != nil {
		return nil, err
	}
	if validLongEnough(awsCreds, o) {
		return awsCreds, nil
	}
	log.Debug("Cached credentials expire too soon, authenticating", "identity", identityName, "min_validity", o.minValidity)
	return nil, nil
}

// authenticateAWS authenticates the identity and returns its AWS credentials.
func authenticateAWS(ctx context.Context, mgr types.AuthManager, identityName string) (*types.AWSCredentials, error) {
	whoami, err := mgr.Authenticate(ctx, identityName)
	if err != nil {
		// The manager already words authentication failures ("authentication failed for identity
		// ...: <cause>"), so prefixing them again would only stutter. Keep the message and let
		// callers match ErrIdentityAuthFailed too.
		if errors.Is(err, errUtils.ErrAuthenticationFailed) {
			return nil, errUtils.MarkAs(err, errUtils.ErrIdentityAuthFailed)
		}
		// A single-cause wrap: the builder lifts the hints, explanation, and context of the cause (for
		// example "identity not found" with its profile hint) so they survive to the formatted output.
		return nil, errUtils.Build(fmt.Errorf("%w '%s'", errUtils.ErrIdentityAuthFailed, identityName)).
			WithCause(err).
			Err()
	}
	if whoami == nil || whoami.Credentials == nil {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, errUtils.ErrIdentityCredentialsNone)
	}
	return requireAWS(whoami.Credentials, identityName)
}

// requireAWS asserts the credentials are AWS credentials.
func requireAWS(creds types.ICredentials, identityName string) (*types.AWSCredentials, error) {
	awsCreds, ok := creds.(*types.AWSCredentials)
	if !ok || awsCreds == nil {
		return nil, fmt.Errorf("%w: identity %q produces %T credentials", errUtils.ErrIdentityNotAWS, identityName, creds)
	}
	return awsCreds, nil
}

// validLongEnough reports whether cached credentials can be reused without authenticating.
// Only temporary credentials qualify: credentials without an expiration (long-lived keys) and
// credentials with an unparseable expiration always trigger authentication.
func validLongEnough(creds *types.AWSCredentials, o *options) bool {
	if creds.Expiration == "" {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, creds.Expiration)
	if err != nil {
		return false
	}
	return expiry.After(o.now().Add(o.minValidity))
}
