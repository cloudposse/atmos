package monorepo

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	gh "github.com/cloudposse/atmos/pkg/github"
)

// expectTrees scripts the two lookups of one verification: the monorepo module, then the successor src.
func expectTrees(resolver *MockTreeResolver, legacy, successor func() (string, error)) {
	gomock.InOrder(
		resolver.EXPECT().TreeSHA(gomock.Any(), LegacyOwner, LegacyRepository, "1.495.0", "modules/aws-sso").DoAndReturn(
			func(context.Context, string, string, string, string) (string, error) { return legacy() },
		),
		resolver.EXPECT().TreeSHA(gomock.Any(), SuccessorOwner, "aws-identity-center", "1.495.0", "src").DoAndReturn(
			func(context.Context, string, string, string, string) (string, error) { return successor() },
		).MaxTimes(1),
	)
}

func sha(value string) func() (string, error) {
	return func() (string, error) { return value, nil }
}

func fail(err error) func() (string, error) {
	return func() (string, error) { return "", err }
}

func TestTreeVerifierVerify(t *testing.T) {
	result, ok := Rewrite("github.com/cloudposse/terraform-aws-components.git//modules/aws-sso?ref=1.495.0")
	require.True(t, ok)

	tests := []struct {
		name      string
		legacy    func() (string, error)
		successor func() (string, error)
		verdict   Verdict
		contains  []string
	}{
		{"equal trees", sha("0123456789abcdef0123"), sha("0123456789abcdef0123"), VerdictIdentical, []string{"identical at 1.495.0", "0123456789ab"}},
		{"different trees", sha("aaaaaaaaaaaaaaaa"), sha("bbbbbbbbbbbbbbbb"), VerdictDiffers, []string{"differs from aws-identity-center/src", "aaaaaaaaaaaa", "bbbbbbbbbbbb"}},
		{"tag missing from the monorepo", fail(fmt.Errorf("%w: x", gh.ErrRefNotFound)), nil, VerdictDiffers, []string{"cloudposse/terraform-aws-components has no tag or ref \"1.495.0\""}},
		{"tag missing from the successor", sha("a"), fail(fmt.Errorf("%w: x", gh.ErrRefNotFound)), VerdictDiffers, []string{"cloudposse-terraform-components/aws-identity-center has no tag or ref \"1.495.0\""}},
		{"directory missing from the successor", sha("a"), fail(fmt.Errorf("%w: o/aws-identity-center has no src at 1.495.0", gh.ErrPathNotFound)), VerdictDiffers, []string{"o/aws-identity-center has no src at 1.495.0"}},
		{"rate limited", fail(errUtils.ErrGitHubRateLimitExceeded), nil, VerdictUnverified, []string{"rate limit", "GITHUB_TOKEN"}},
		{"credentials rejected", fail(fmt.Errorf("wrapped: %w", errUtils.ErrAuthenticationFailed)), nil, VerdictUnverified, []string{"rejected the credentials"}},
		{"network failure", sha("a"), fail(errors.New("dial tcp: no route to host")), VerdictUnverified, []string{"could not read cloudposse-terraform-components/aws-identity-center", "no route to host"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewMockTreeResolver(gomock.NewController(t))
			successor := tt.successor
			if successor == nil {
				successor = sha("unused")
			}
			expectTrees(resolver, tt.legacy, successor)

			got := NewTreeVerifier(resolver).Verify(context.Background(), &result, "1.495.0")

			assert.Equal(t, tt.verdict, got.Verdict, got.Reason)
			for _, part := range tt.contains {
				assert.Contains(t, got.Reason, part)
			}
		})
	}
}

func TestNewTreeVerifierDefaultsToGitHub(t *testing.T) {
	assert.IsType(t, githubTrees{}, NewTreeVerifier(nil).trees)
}
