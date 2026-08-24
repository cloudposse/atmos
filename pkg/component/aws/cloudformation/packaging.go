package cloudformation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
	artifact "github.com/cloudposse/atmos/pkg/ci/artifact"
	s3store "github.com/cloudposse/atmos/pkg/ci/artifact/s3"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

// templateInlineSizeLimit is CloudFormation's inline TemplateBody limit (51,200
// bytes). Above this, the template must be uploaded and referenced via
// TemplateURL instead — this is what `aws cloudformation package`/Rain's `pkg`
// do, and what this file's uploadPackage does for Phase 1.
const templateInlineSizeLimit = 51200

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

	backend, err := newS3Backend(atmosConfig, info, s3Target)
	if err != nil {
		return nil, err
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
func newS3Backend(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, s3Target *targetS3Config) (artifact.Backend, error) {
	opts := artifact.StoreOptions{
		Type: "s3",
		Options: map[string]any{
			"bucket": s3Target.Bucket,
			"prefix": s3Target.Prefix,
			"region": s3Target.Region,
		},
		AtmosConfig: atmosConfig,
	}

	if info.Identity != "" {
		if authManager, ok := info.AuthManager.(types.AuthManager); ok {
			opts.Identity = info.Identity
			opts.Resolver = authbridge.NewResolver(authManager, info)
		}
	}

	return s3store.NewStore(opts)
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

// packageURL constructs the s3:// URL for a packaged template. CreateChangeSet's
// TemplateURL parameter also accepts virtual-hosted-style https URLs; s3:// is
// used here since it's unambiguous across regions/partitions.
func packageURL(s3Target *targetS3Config, name string) string {
	return fmt.Sprintf("s3://%s/%s", s3Target.Bucket, name)
}
