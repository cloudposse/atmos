package publicaccess

import (
	"fmt"
	"net/url"
	"strings"

	"al.essio.dev/pkg/shellescape"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

// PublicationError reports a verification failure without implying the upload was rolled back.
func PublicationError(repository, reference string, cause error) error {
	defer perf.Track(nil, "publicaccess.PublicationError")()

	builder := errUtils.Build(fmt.Errorf("%s (reference %s) was published but public access verification failed: %w", repository, reference, cause)).
		WithHint("The uploaded content remains in the registry. After resolving access, rerun the same `atmos plugin publish`, `atmos plugin release`, or `atmos plugin catalog sync` command with identical inputs.")
	repo, err := pluginoci.ParseDestination(repository)
	if err == nil && repo.RegistryStr() == "ghcr.io" {
		parts := strings.SplitN(repo.RepositoryStr(), "/", 2)
		if len(parts) == 2 {
			builder.WithHintf("GHCR creates new packages private. For an organization package, open https://github.com/orgs/%s/packages/container/package/%s/settings and select Danger Zone → Change visibility → Public. For personal packages, use your profile’s Packages tab → Package settings. Source repository labels do not set visibility. If it is already public, inspect the underlying access or integrity error.", url.PathEscape(parts[0]), url.PathEscape(parts[1]))
		}
	}
	builder.WithHintf("Check anonymous access with: %s", shellescape.QuoteCommand([]string{"atmos", "plugin", "verify-public", repository, "--reference", reference}))
	return builder.Err()
}
