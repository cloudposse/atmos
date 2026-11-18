package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

func staticPolicy() artifact.VerificationPolicy {
	return artifact.VerificationPolicy{
		RequireSignature: true,
		TrustSets: []artifact.TrustSet{
			{Identities: []artifact.TrustedIdentity{{Identity: "release@example.com", Issuer: "https://issuer.example.com"}}, Keys: []string{"release.pub"}},
		},
	}
}

func configuredInfo() service.RepositoryInfo {
	return service.RepositoryInfo{
		Capabilities: artifact.Capabilities{Read: true, Packagings: []artifact.Packaging{artifact.PackagingTar, artifact.PackagingZip}, LockScopes: []artifact.LockScope{artifact.LockScopeHost}},
		Defaults: artifact.Metadata{
			Description: "defaults",
			Labels:      map[string]string{"team": "platform"},
			Provenance:  &artifact.Provenance{CI: &artifact.CIProvenance{Provider: "github"}},
		},
		Policy: staticPolicy(),
	}
}

func reportedCapabilities() artifact.Capabilities {
	return artifact.Capabilities{Read: true, Write: true, Packagings: []artifact.Packaging{artifact.PackagingTgz, artifact.PackagingNone}, SubjectKinds: []artifact.SubjectKind{artifact.SubjectRecord}}
}

// TestStaticRepositories_DescribeReturnsIndependentCopies proves a caller editing what Describe
// returned cannot change the table or the repository, which every later Describe would serve.
func TestStaticRepositories_DescribeReturnsIndependentCopies(t *testing.T) {
	tests := []struct {
		name string
		// info builds the table entry; an empty one falls back to what the repository reports.
		info   func() service.RepositoryInfo
		mutate func(info *service.RepositoryInfo)
	}{
		{"defaults labels and provenance", configuredInfo, func(info *service.RepositoryInfo) {
			info.Defaults.Labels["team"] = "scribble"
			info.Defaults.Provenance.CI.Provider = "scribble"
		}},
		{"configured capabilities", configuredInfo, func(info *service.RepositoryInfo) {
			info.Capabilities.Packagings[0] = artifact.PackagingNone
			info.Capabilities.LockScopes[0] = artifact.LockScopeRepository
		}},
		{"policy trust sets", configuredInfo, func(info *service.RepositoryInfo) {
			info.Policy.TrustSets[0].Identities[0].Identity = "scribble"
			info.Policy.TrustSets[0].Keys[0] = "scribble"
		}},
		{"capabilities reported by the repository", func() service.RepositoryInfo { return service.RepositoryInfo{} }, func(info *service.RepositoryInfo) {
			info.Capabilities.Packagings[0] = artifact.PackagingZip
			info.Capabilities.SubjectKinds[0] = "scribble"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The repository hands out the same backing slices on every call, as a cached description would.
			build := func() (*service.StaticRepositories, map[string]service.RepositoryInfo, *artifact.Capabilities) {
				caps := reportedCapabilities()
				repo := artifact.NewMockRepository(gomock.NewController(t))
				repo.EXPECT().Capabilities().DoAndReturn(func() artifact.Capabilities { return caps }).AnyTimes()
				table := map[string]service.RepositoryInfo{"repo": tc.info()}
				return &service.StaticRepositories{Repositories: map[string]artifact.Repository{"repo": repo}, Info: table}, table, &caps
			}
			untouched, _, _ := build()
			want, err := untouched.Describe("repo")
			require.NoError(t, err)

			static, table, repoCaps := build()
			first, err := static.Describe("repo")
			require.NoError(t, err)
			tc.mutate(&first)

			second, err := static.Describe("repo")
			require.NoError(t, err)
			assert.Equal(t, want, second, "a later description is not affected by edits to an earlier one")
			assert.Equal(t, tc.info(), table["repo"], "the configured table entry is untouched")
			assert.Equal(t, reportedCapabilities(), *repoCaps, "the repository's own capabilities are untouched")
		})
	}
}
