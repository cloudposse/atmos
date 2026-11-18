package downloader

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: the tests below rely on the AtmosConfiguration struct.
var _ = schema.AtmosConfiguration{}

// registerTestResolver installs resolver for the test and restores the previous one afterwards.
func registerTestResolver(t *testing.T, resolver IdentityEnvironmentResolver) {
	t.Helper()
	identityResolverMu.RLock()
	previous := identityResolver
	identityResolverMu.RUnlock()
	RegisterIdentityEnvironmentResolver(resolver)
	t.Cleanup(func() { RegisterIdentityEnvironmentResolver(previous) })
}

func TestResolveIdentityName_Precedence(t *testing.T) {
	tests := []struct {
		name           string
		override       string
		entry          string
		sectionDefault string
		want           string
	}{
		{name: "nothing configured", want: ""},
		{name: "section default only", sectionDefault: "default-id", want: "default-id"},
		{name: "entry beats section default", entry: "entry-id", sectionDefault: "default-id", want: "entry-id"},
		{name: "flag beats entry and section default", override: "flag-id", entry: "entry-id", sectionDefault: "default-id", want: "flag-id"},
		{name: "flag alone", override: "flag-id", want: "flag-id"},
		{name: "disabled flag turns identities off", override: identityFlagDisabled, entry: "entry-id", sectionDefault: "default-id", want: ""},
		{name: "bare --identity never selects interactively", override: identityFlagSelect, entry: "entry-id", sectionDefault: "default-id", want: "entry-id"},
		{name: "bare --identity falls through to the default", override: identityFlagSelect, sectionDefault: "default-id", want: "default-id"},
		{name: "whitespace only entry is unset", entry: "  ", sectionDefault: "default-id", want: "default-id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithIdentityOverride(context.Background(), tt.override)
			assert.Equal(t, tt.want, ResolveIdentityName(ctx, tt.entry, tt.sectionDefault))
		})
	}
}

func TestWithIdentity_NoIdentityLeavesContextAndSkipsResolver(t *testing.T) {
	var calls atomic.Int32
	registerTestResolver(t, func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		calls.Add(1)
		return []string{"X=1"}, nil
	})

	parent := context.Background()
	got, err := WithIdentity(parent, &schema.AtmosConfiguration{}, "", "")

	require.NoError(t, err)
	assert.Equal(t, parent, got, "a source without an identity behaves exactly as before")
	assert.Nil(t, EnvironmentFromContext(got))
	assert.Zero(t, calls.Load())
}

func TestWithIdentity_ResolvesEffectiveIdentityIntoFetchEnvironment(t *testing.T) {
	var seen []string
	registerTestResolver(t, func(_ context.Context, _ *schema.AtmosConfiguration, identity string) ([]string, error) {
		seen = append(seen, identity)
		return []string{"IDENTITY_NAME=" + identity}, nil
	})

	tests := []struct {
		name           string
		override       string
		entry          string
		sectionDefault string
		want           string
	}{
		{name: "section default", sectionDefault: "default-id", want: "default-id"},
		{name: "entry", entry: "entry-id", sectionDefault: "default-id", want: "entry-id"},
		{name: "flag", override: "flag-id", entry: "entry-id", sectionDefault: "default-id", want: "flag-id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen = nil
			ctx := WithIdentityOverride(context.Background(), tt.override)

			got, err := WithIdentity(ctx, &schema.AtmosConfiguration{}, tt.entry, tt.sectionDefault)

			require.NoError(t, err)
			assert.Equal(t, []string{tt.want}, seen)
			assert.Equal(t, []string{"IDENTITY_NAME=" + tt.want}, EnvironmentFromContext(got))
		})
	}
}

func TestWithIdentity_ResolverFailureIsReturned(t *testing.T) {
	boom := errors.New("not logged in")
	registerTestResolver(t, func(context.Context, *schema.AtmosConfiguration, string) ([]string, error) {
		return nil, boom
	})

	_, err := WithIdentity(context.Background(), &schema.AtmosConfiguration{}, "ops", "")

	require.ErrorIs(t, err, boom)
}

func TestWithIdentity_NoResolverRegistered(t *testing.T) {
	registerTestResolver(t, nil)

	_, err := WithIdentity(context.Background(), &schema.AtmosConfiguration{}, "ops", "")

	require.ErrorIs(t, err, errUtils.ErrRemoteIdentityResolverUnavailable)
}
