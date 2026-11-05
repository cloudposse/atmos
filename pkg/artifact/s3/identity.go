package s3

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/store"
)

// identityAdapter lets the S3 store resolve an Atmos identity through the repository's lazy
// resolver. The generic resolver returns a backend-specific value; the S3 backend accepts only AWS
// authentication configuration, and anything else fails closed.
type identityAdapter struct {
	resolve artifact.IdentityResolver
}

// ResolveAWSAuthContext implements the store's AuthResolver.
func (a identityAdapter) ResolveAWSAuthContext(ctx context.Context, identity string) (*store.AWSAuthConfig, error) {
	defer perf.Track(nil, "s3.identityAdapter.ResolveAWSAuthContext")()

	value, err := a.resolve(ctx, identity)
	if err != nil {
		return nil, err
	}
	switch typed := value.(type) {
	case *store.AWSAuthConfig:
		if typed != nil {
			return typed, nil
		}
	case store.AWSAuthConfig:
		return &typed, nil
	}
	return nil, fmt.Errorf("%w: identity %q did not resolve to AWS credentials (got %T)", errUtils.ErrBlobIdentityUnresolved, identity, value)
}
