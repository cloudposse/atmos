// Package credentialprocess produces AWS process-credential documents for Atmos identities.
//
// It backs `atmos aws credential-process` and `atmos auth env --format=credential-process`, so
// both entry points emit byte-identical output. The document follows the AWS SDK
// `credential_process` contract:
// https://docs.aws.amazon.com/sdkref/latest/guide/feature-process-credentials.html.
package credentialprocess

import (
	"context"
	"fmt"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultMinValidity is the default remaining lifetime cached credentials must have to be
// reused without authenticating again. The AWS CLI runs credential_process once per invocation,
// so reusing credentials that are still valid for a while keeps every `aws` command fast.
const DefaultMinValidity = 15 * time.Minute

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
// `atmos auth login` would. Cached credentials without an expiration are never printed directly:
// for an IAM user they are the long-lived access keys, and authenticating exchanges them for a
// session (honoring MFA) instead of handing the keys to the caller. Auto-triggered integrations (for example
// kubeconfig or ECR login) are skipped because this runs as a non-interactive helper.
func Produce(ctx context.Context, mgr types.AuthManager, identityName string, opts ...Option) ([]byte, error) {
	defer perf.Track(nil, "credentialprocess.Produce")()

	o := options{minValidity: DefaultMinValidity, now: time.Now}
	for _, opt := range opts {
		opt(&o)
	}

	ctx = auth.ContextWithSkipIntegrations(ctx)

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

// WriteFile writes the rendered document to path, replacing any existing content. The file
// holds secrets, so it is created owner-readable only and an existing file is tightened to the
// same mode.
func WriteFile(path string, doc []byte) error {
	defer perf.Track(nil, "credentialprocess.WriteFile")()

	if err := os.WriteFile(path, []byte(Render(doc)), fileMode); err != nil {
		return errUtils.Build(errUtils.ErrWriteFile).
			WithCause(err).
			WithContext("path", path).
			Err()
	}
	// WriteFile applies the mode only when it creates the file.
	if err := os.Chmod(path, fileMode); err != nil {
		return errUtils.Build(errUtils.ErrWriteFile).
			WithCause(err).
			WithContext("path", path).
			Err()
	}
	return nil
}

// ResolveIdentity resolves the identity to produce credentials for. A concrete name is returned
// as-is, the interactive-selection sentinel (--identity without a value) opens the selector
// (which fails without a TTY), and an empty name falls back to the configured default identity.
func ResolveIdentity(mgr types.AuthManager, identityName string) (string, error) {
	defer perf.Track(nil, "credentialprocess.ResolveIdentity")()

	if identityName == cfg.IdentityFlagSelectValue {
		return auth.ResolveSelectedIdentity(mgr, identityName, cfg.IdentityFlagSelectValue)
	}
	if identityName != "" {
		return identityName, nil
	}

	defaultIdentity, err := mgr.GetDefaultIdentity(false)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoDefaultIdentity, err)
	}
	return defaultIdentity, nil
}

// resolveCredentials returns reusable cached AWS credentials, or authenticates for fresh ones.
func resolveCredentials(ctx context.Context, mgr types.AuthManager, identityName string, o *options) (*types.AWSCredentials, error) {
	if cached, err := mgr.GetCachedCredentials(ctx, identityName); err != nil {
		log.Debug("No valid cached credentials, authenticating", "identity", identityName, "error", err)
	} else if cached != nil && cached.Credentials != nil {
		awsCreds, err := requireAWS(cached.Credentials, identityName)
		if err != nil {
			return nil, err
		}
		if validLongEnough(awsCreds, o) {
			return awsCreds, nil
		}
		log.Debug("Cached credentials expire too soon, authenticating", "identity", identityName, "min_validity", o.minValidity)
	}

	whoami, err := mgr.Authenticate(ctx, identityName)
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapWithNameAndCauseFormat, errUtils.ErrIdentityAuthFailed, identityName, err)
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
