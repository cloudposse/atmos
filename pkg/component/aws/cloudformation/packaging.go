package cloudformation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	artifact "github.com/cloudposse/atmos/pkg/ci/artifact"
	_ "github.com/cloudposse/atmos/pkg/ci/artifact/s3" // Registers the "aws/s3" artifact.Backend factory used via artifact.NewBackend below.
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
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
	// URL is the https TemplateURL CreateChangeSet references.
	URL string
	// S3URI is the same object as an s3://bucket/key URI, for humans and tooling.
	S3URI string
	// SHA256 is the template's hex-encoded SHA-256 digest.
	SHA256 string
	// Reused reports that the content-addressed object already existed, so no
	// upload (and no new S3 object version) was made.
	Reused bool
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

// packagingRequest bundles packageTemplate's inputs to stay under this repo's
// 5-argument function limit.
type packagingRequest struct {
	ProvisionSection map[string]any
	Selected         *target.SelectedTarget
	Spec             *stackSpec
	Summary          map[string]any
	// MayProvision reports whether this operation may create the packaging
	// bucket when provision.backend.enabled is true. Only apply/deploy provision
	// infrastructure; validate, diff and changeset create must not.
	MayProvision bool
}

// packageTemplate uploads the template to the packaging target when it exceeds
// the inline size limit (or an aws/s3 target was selected directly), points
// Spec.TemplateURL at the uploaded object, and records where it went in
// Summary under package_url (the https TemplateURL), package_s3_uri
// (s3://bucket/key), package_sha256, and package_reused (true when the
// content-addressed object already existed and no upload was made).
func packageTemplate(octx *opContext, req *packagingRequest) error {
	if !needsPackaging(req.Spec.TemplateBody) && req.Selected.Kind != kindAwsS3 {
		return nil
	}

	s3Target, err := resolvePackagingTarget(req.ProvisionSection, req.Selected)
	if err != nil {
		return err
	}

	args := autoProvisionArgs{
		AtmosConfig:     octx.AtmosConfig,
		S3Target:        s3Target,
		ComponentConfig: octx.Info.ComponentSection,
		AuthContext:     octx.Info.AuthContext,
		Component:       octx.Info.ComponentFromArg,
		Stack:           octx.Info.Stack,
	}
	if req.MayProvision {
		err = autoProvisionBackendIfEnabled(octx.Ctx, args)
	} else {
		err = requireBackendExistsIfEnabled(octx.Ctx, args)
	}
	if err != nil {
		return err
	}

	pkg, err := uploadPackage(octx.Ctx, octx.AtmosConfig, octx.Info, s3Target, req.Spec.TemplateBody)
	if err != nil {
		return err
	}
	req.Summary["package_url"] = pkg.URL
	req.Summary["package_s3_uri"] = pkg.S3URI
	req.Summary["package_sha256"] = pkg.SHA256
	req.Summary["package_reused"] = pkg.Reused
	req.Spec.TemplateURL = pkg.URL
	return nil
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
	name := packageObjectName("", info, digest)
	//nolint:forbidigo // S3 object keys use forward slashes on every OS, matching the artifact backend.
	key := path.Join(normalizeS3Prefix(s3Target.Prefix), name)

	// The key is content-addressed, so an object whose sidecar records the same
	// digest is byte-identical. Re-uploading it would only pile up object versions
	// on every run of a versioned bucket.
	reused := packageAlreadyPublished(ctx, backend, name, digest)
	if !reused {
		metadata := &artifact.Metadata{
			Stack:        info.Stack,
			Component:    info.ComponentFromArg,
			SHA256:       digest,
			CreatedAt:    time.Now(),
			AtmosVersion: "",
		}
		if err := backend.Upload(ctx, name, strings.NewReader(templateBody), int64(len(templateBody)), metadata); err != nil {
			return nil, fmt.Errorf("packaging template to s3://%s/%s: %w", s3Target.Bucket, key, err)
		}
	}

	return &packageUpload{
		URL:    packageURL(s3Target, key),
		S3URI:  fmt.Sprintf("s3://%s/%s", s3Target.Bucket, key),
		SHA256: digest,
		Reused: reused,
	}, nil
}

// packageAlreadyPublished reports whether the packaged template object already
// exists with a metadata sidecar recording the same digest. Any lookup failure
// (including a missing object, a missing sidecar, or insufficient permission to
// read it) reports false, so the caller uploads and surfaces the real error.
func packageAlreadyPublished(ctx context.Context, backend artifact.Backend, name, digest string) bool {
	metadata, err := backend.GetMetadata(ctx, name)
	if err != nil {
		if !errors.Is(err, errUtils.ErrArtifactNotFound) {
			log.Debug("Could not check for an existing packaged template; uploading", "name", name, "error", err)
		}
		return false
	}
	return metadata != nil && metadata.SHA256 == digest
}

// normalizeS3Prefix trims leading and trailing slashes so a configured prefix
// such as "/lead" or "lead/" never yields an S3 key that starts with "/" or an
// empty path segment.
func normalizeS3Prefix(prefix string) string {
	return strings.Trim(prefix, "/")
}

// targetS3Config is the resolved `kind: aws/s3` provision target configuration.
//
// The yaml/json tags give `backend describe`/`list` machine-readable output the
// snake_case keys the rest of this component type's output uses.
type targetS3Config struct {
	Name   string `json:"name" yaml:"name"`
	Bucket string `json:"bucket" yaml:"bucket"`
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	Region string `json:"region,omitempty" yaml:"region,omitempty"`
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
			"prefix": normalizeS3Prefix(s3Target.Prefix),
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

// packageURL constructs the https:// URL CreateChangeSet's TemplateURL
// parameter requires. AWS rejects a bare s3:// URI here (TemplateURL must be
// an S3 or Systems Manager document URL starting with https://), so a region
// is mandatory -- s3ConfigFromTarget enforces that before this is ever
// called. GovCloud/China partitions (a different DNS suffix than
// amazonaws.com) aren't handled: nothing else in this codebase resolves AWS
// partition yet, so extending that is left to a future change if/when it's
// needed rather than guessed at here.
//
// Addressing style: virtual-hosted-style (https://<bucket>.s3.<region>.amazonaws.com/<key>)
// is used by default, per AWS's own current guidance. But a bucket name
// containing dots (a legal S3 bucket name, e.g. "my.bucket.name") breaks TLS
// certificate validation under virtual-hosted-style addressing -- the
// wildcard cert for *.s3.<region>.amazonaws.com covers exactly one label, not
// the multi-label "my.bucket.name.s3.<region>.amazonaws.com" host that would
// result. Path-style addressing (https://s3.<region>.amazonaws.com/<bucket>/<key>)
// sidesteps that by keeping the bucket out of the hostname entirely, so it's
// used specifically -- and only -- for dotted bucket names.
//
// The object key is percent-escaped via net/url rather than interpolated
// raw: name can contain characters from the stack/component/prefix (spaces,
// "#", "?", etc.) that are otherwise not valid unescaped in a URL, or that
// would silently point CreateChangeSet at the wrong object (e.g. an
// unescaped "#" truncates the path at a URL fragment). The url.URL type's
// Path field escapes each path segment while leaving the "/" separators
// intact, which is exactly the key's own directory structure
// (stack/component/template).
func packageURL(s3Target *targetS3Config, name string) string {
	u := url.URL{Scheme: "https", Path: "/" + name}
	if strings.Contains(s3Target.Bucket, ".") {
		u.Host = fmt.Sprintf("s3.%s.amazonaws.com", s3Target.Region)
		u.Path = "/" + s3Target.Bucket + "/" + name
	} else {
		u.Host = fmt.Sprintf("%s.s3.%s.amazonaws.com", s3Target.Bucket, s3Target.Region)
	}
	return u.String()
}
