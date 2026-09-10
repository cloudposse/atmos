package cloudformation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	artifact "github.com/cloudposse/atmos/pkg/ci/artifact"
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/s3" // Registers the "aws/s3" artifact.Backend factory used via artifact.NewBackend below.
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

// templateInlineSizeLimit is CloudFormation's inline TemplateBody limit (51,200
// bytes). Above this, the template must be uploaded and referenced via
// TemplateURL instead — this is what `aws cloudformation package`/Rain's `pkg`
// do, and what this file's uploadPackage does for Phase 1.
const templateInlineSizeLimit = 51200

// newS3BackendFunc is a seam for testing: uploadPackage calls through this
// var (rather than newS3Backend directly) so tests can inject a fake
// artifact.Backend and exercise uploadPackage's digest/name/error-wrapping
// logic without making a real network call to S3 (constructing the real
// backend is fast/local, but Backend.Upload always attempts a live AWS API
// call, which is exactly the kind of integration dependency CLAUDE.md's
// Testing Strategy says to mock rather than exercise for real).
var newS3BackendFunc = newS3Backend

// packageUpload is the outcome of uploading a template to a `kind: aws/s3`
// provision target: the URL CreateChangeSet's TemplateURL can reference, and a
// SHA-256 digest for provenance.
type packageUpload struct {
	URL    string
	SHA256 string
}

// needsPackaging reports whether the template body exceeds CloudFormation's
// inline size limit and must be uploaded to S3 rather than passed inline.
//
// Local-asset rewriting (Lambda source, nested-stack templates referenced by
// relative path, etc. — what `aws cloudformation package` does beyond simple
// size-driven upload) is not implemented in Phase 1; templates that need it
// should reference already-published S3 locations directly until a future
// phase closes this gap.
func needsPackaging(templateBody string) bool {
	return len(templateBody) > templateInlineSizeLimit
}

// uploadPackage uploads the template body to the selected `kind: aws/s3`
// provision target, implementing the PRD's "narrow seam" contract: this
// function (not pkg/ci/artifact) computes the digest and constructs the URL,
// so packaging introduces no new artifact-kind vocabulary of its own — it
// wraps the existing aws/s3 Backend.Upload (which itself returns only an
// error, no URL/digest).
func uploadPackage(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, s3Target *targetS3Config, templateBody string) (*packageUpload, error) {
	defer perf.Track(atmosConfig, "cloudformation.uploadPackage")()

	backend, err := newS3BackendFunc(atmosConfig, info, s3Target)
	if err != nil {
		return nil, fmt.Errorf("creating S3 packaging backend for bucket %q: %w", s3Target.Bucket, err)
	}

	sum := sha256.Sum256([]byte(templateBody))
	digest := hex.EncodeToString(sum[:])
	name := packageObjectName(s3Target.Prefix, info, digest)

	metadata := &artifact.Metadata{
		Stack:        info.Stack,
		Component:    info.ComponentFromArg,
		SHA256:       digest,
		CreatedAt:    time.Now(),
		AtmosVersion: "",
	}

	if err := backend.Upload(ctx, name, strings.NewReader(templateBody), int64(len(templateBody)), metadata); err != nil {
		return nil, fmt.Errorf("packaging template to s3://%s/%s: %w", s3Target.Bucket, name, err)
	}

	return &packageUpload{
		URL:    packageURL(s3Target, name),
		SHA256: digest,
	}, nil
}

// targetS3Config is the resolved `kind: aws/s3` provision target configuration.
type targetS3Config struct {
	Name   string
	Bucket string
	Prefix string
	Region string
}

// newS3Backend constructs the aws/s3 artifact backend for the selected packaging
// target, authenticated via the active identity (never a bare ambient
// credential chain — see environment.go for the same in-process principle
// applied to the CloudFormation client itself).
//
// Critically, "the active identity" is not the same thing as "an explicit
// identity: override": most stacks authenticate via a default identity
// (auth.identities.<name>.default: true) and never set info.Identity at all.
// The activeIdentityName helper below resolves the identity name whichever
// way it was activated — explicit override or default — the same way
// environment.go's awsAuthContextFrom/buildAWSConfig already trust whatever
// identity has resolved onto info for the CloudFormation client itself,
// rather than requiring info.Identity to be a non-empty explicit string.
func newS3Backend(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, s3Target *targetS3Config) (artifact.Backend, error) {
	opts := artifact.StoreOptions{
		// Type must match the registered artifact.Backend factory key ("aws/s3",
		// not "s3" — see pkg/ci/artifact/s3/store.go's storeName const): this is
		// now a real registry lookup via artifact.NewBackend below, whereas it was
		// previously inert metadata when this function called s3store.NewStore
		// directly.
		Type: "aws/s3",
		Options: map[string]any{
			"bucket": s3Target.Bucket,
			"prefix": s3Target.Prefix,
			"region": s3Target.Region,
		},
		AtmosConfig: atmosConfig,
	}

	if identityName := activeIdentityName(info); identityName != "" {
		authManager, ok := info.AuthManager.(types.AuthManager)
		if !ok {
			// Fail loudly rather than silently falling back to the artifact
			// registry's default (ambient) credential chain — that would upload
			// the packaged template using whatever AWS credentials happen to be
			// available in the environment instead of the identity that is
			// actually active for this command (explicit override or default).
			return nil, fmt.Errorf("%w: identity %q", errUtils.ErrAwsCloudFormationIdentityResolutionFailed, identityName)
		}
		opts.Identity = identityName
		opts.Resolver = authbridge.NewResolver(authManager, info)
	}

	// Route through the artifact registry's NewBackend (rather than calling
	// s3store.NewStore directly) so that, when opts.Resolver is set, the
	// registry's SetAuthContext wiring (registry.go) actually reaches the
	// backend. Calling s3store.NewStore directly — as this used to — built
	// opts.Resolver into the call but never wired it into the Store, silently
	// leaving the identity-aware client uninitialized until s3store's own
	// nil-resolver fallback quietly reached for the ambient chain instead.
	return artifact.NewBackend(opts)
}

// activeIdentityName returns the identity name whose credentials should
// authenticate the S3 upload: an explicit component-level identity: override
// (info.Identity) when set, otherwise the identity that already authenticated
// as this command's active/default identity, if any. The active identity's
// name is recovered from AuthContext.AWS.Profile, which the auth system always
// sets to the identity name (see pkg/auth/cloud/aws/setup.go's
// Profile: params.IdentityName) — regardless of whether that identity was
// selected via an explicit override or a configured default. Returns "" when
// no identity is active at all, in which case the caller falls back to the
// ambient AWS credential chain (e.g. local/unauthenticated usage where no
// Atmos auth is configured).
func activeIdentityName(info *schema.ConfigAndStacksInfo) string {
	if info == nil {
		return ""
	}
	if info.Identity != "" {
		return info.Identity
	}
	if info.AuthContext != nil && info.AuthContext.AWS != nil {
		return info.AuthContext.AWS.Profile
	}
	return ""
}

// packageObjectName builds a deterministic, content-addressed S3 key for the
// packaged template.
func packageObjectName(prefix string, info *schema.ConfigAndStacksInfo, digest string) string {
	name := fmt.Sprintf("%s/%s/template-%s.yaml", info.Stack, info.ComponentFromArg, digest[:12])
	if prefix == "" {
		return name
	}
	return strings.TrimSuffix(prefix, "/") + "/" + name
}

// packageURL constructs the virtual-hosted-style https:// URL CreateChangeSet's
// TemplateURL parameter requires. AWS rejects a bare s3:// URI here (TemplateURL
// must be an S3 or Systems Manager document URL starting with https://), so a
// region is mandatory -- s3ConfigFromTarget enforces that before this is ever
// called. GovCloud/China partitions (a different DNS suffix than
// amazonaws.com) aren't handled: nothing else in this codebase resolves AWS
// partition yet, so extending that is left to a future change if/when it's
// needed rather than guessed at here.
func packageURL(s3Target *targetS3Config, name string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s3Target.Bucket, s3Target.Region, name)
}
