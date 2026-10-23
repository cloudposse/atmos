package guard

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestAuthenticationGate(t *testing.T) {
	input := AuthInput{Identity: "dev", Provider: "prod", Identities: []string{"prod-admin", "dev"}}
	for _, tc := range []struct {
		name    string
		enabled bool
		lookup  error
		want    error
		calls   int
	}{
		{"disabled", false, nil, nil, 0},
		{"lookup error", false, errUtils.ErrPolicyInvalid, errUtils.ErrPolicyInvalid, 0},
		{"denied", true, nil, errUtils.ErrPolicyDenied, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			installGate(t, func(kind string) (bool, error) {
				require.Equal(t, "auth", kind)
				return tc.enabled, tc.lookup
			}, func(_ context.Context, kind string, raw json.RawMessage) error {
				calls++
				require.Equal(t, "auth", kind)
				require.JSONEq(t, `{"identity":"dev","provider":"prod","identities":["prod-admin","dev"]}`, string(raw))
				return errUtils.ErrPolicyDenied
			})
			err := Authenticate(t.Context(), input)
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, tc.calls, calls)
		})
	}
}
