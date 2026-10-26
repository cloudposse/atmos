package guard

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AuthInput identifies the resolved credential chain without exposing credentials.
type AuthInput struct {
	Identity   string   `json:"identity"`
	Provider   string   `json:"provider"`
	Identities []string `json:"identities"`
}

// Authenticate checks the complete chain before credential acquisition or cache access.
func Authenticate(ctx context.Context, input AuthInput) error {
	defer perf.Track(nil, "guard.Authenticate")()

	enabled, err := Enabled("auth")
	if err != nil || !enabled {
		return err
	}
	return evaluate(ctx, "auth", input)
}
