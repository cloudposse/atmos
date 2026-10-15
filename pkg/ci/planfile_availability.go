package ci

import (
	"context"
	"os"
	"time"

	awsauth "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

const planfileCredentialLookupTimeout = 2 * time.Second

// planfileEnvironmentChecker checks credential availability before selecting a
// priority entry. Explicit defaults bypass these checks in artifact.SelectStore.
// This does not probe bucket permissions or retry uploads in another store.
type planfileEnvironmentChecker struct {
	options artifact.StoreOptions
}

func (c planfileEnvironmentChecker) IsAvailable(ctx context.Context) bool {
	defer perf.Track(nil, "ci.planfileEnvironmentChecker.IsAvailable")()
	switch c.options.Type {
	case "github/artifacts":
		return os.Getenv("ATMOS_CI_GITHUB_TOKEN") != "" || os.Getenv("GITHUB_TOKEN") != "" || os.Getenv("GH_TOKEN") != ""
	case "aws/s3":
		// Atmos resolves an explicitly selected identity when the backend is used.
		// Ambient credentials must not override or exclude that identity.
		if c.options.Identity != "" {
			return true
		}
		return ambientAWSCredentialsAvailable(ctx)
	default:
		return true
	}
}

func ambientAWSCredentialsAvailable(ctx context.Context) bool {
	// Bound SDK credential-process/metadata lookups during priority selection.
	ctx, cancel := context.WithTimeout(ctx, planfileCredentialLookupTimeout)
	defer cancel()
	_, err := awsauth.LoadDefaultAWSCredentials(ctx)
	return err == nil
}
