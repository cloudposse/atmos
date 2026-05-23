package security

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgsecurity "github.com/cloudposse/atmos/pkg/aws/security"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withFactories swaps the package-level factories for the duration of the test.
// A nil mockClient combined with a non-nil clientErr exercises the
// proClientFactory failure path.
func withFactories(t *testing.T, mockRepo git.GitRepoInterface, mockClient pro.APIClient, clientErr error) {
	t.Helper()
	origRepo := gitRepoFactory
	origClient := proClientFactory
	gitRepoFactory = func() git.GitRepoInterface { return mockRepo }
	proClientFactory = func(*schema.AtmosConfiguration) (pro.APIClient, error) {
		if clientErr != nil {
			return nil, clientErr
		}
		return mockClient, nil
	}
	t.Cleanup(func() {
		gitRepoFactory = origRepo
		proClientFactory = origClient
	})
}

// newUploadTestReport mirrors the fixture in report_renderer_test.go in a form
// callable from cmd-package tests (no internal access required).
func newUploadTestReport() *pkgsecurity.Report {
	return &pkgsecurity.Report{
		GeneratedAt:   time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		Stack:         "tenant1-ue1-prod",
		Component:     "vpc",
		TotalFindings: 1,
		SeverityCounts: map[pkgsecurity.Severity]int{
			pkgsecurity.SeverityHigh: 1,
		},
		MappedCount: 1,
		Findings: []pkgsecurity.Finding{
			{
				ID:           "f1",
				Title:        "Open security group",
				Severity:     pkgsecurity.SeverityHigh,
				Source:       pkgsecurity.SourceSecurityHub,
				ResourceARN:  "arn:aws:ec2:us-east-1:111122223333:security-group/sg-1",
				ResourceType: "AwsEc2SecurityGroup",
				Mapping: &pkgsecurity.ComponentMapping{
					Stack:         "tenant1-ue1-prod",
					Component:     "vpc",
					ComponentPath: "components/terraform/vpc",
					Mapped:        true,
					Confidence:    pkgsecurity.ConfidenceHigh,
					Method:        "tag",
				},
			},
		},
	}
}

func TestUploadReportToAtmosPro_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := git.NewMockGitRepoInterface(ctrl)
	repo.EXPECT().GetLocalRepoInfo().Return(&git.RepoInfo{
		RepoUrl:   "https://github.com/org/repo",
		RepoName:  "repo",
		RepoOwner: "org",
		RepoHost:  "github.com",
	}, nil)
	repo.EXPECT().GetCurrentCommitSHA().Return("deadbeef", nil)

	client := pro.NewMockAPIClient(ctrl)
	var capturedDTO *dtos.SecurityFindingsUploadRequest
	client.EXPECT().UploadSecurityFindings(gomock.Any()).
		DoAndReturn(func(dto *dtos.SecurityFindingsUploadRequest) error {
			capturedDTO = dto
			return nil
		})

	withFactories(t, repo, client, nil)

	cfg := &schema.AtmosConfiguration{}
	report := newUploadTestReport()

	require.NoError(t, uploadReportToAtmosPro(cfg, report, "tenant1-ue1-prod", "vpc"))
	require.NotNil(t, capturedDTO)
	assert.Equal(t, "sarif", capturedDTO.Format)
	assert.Equal(t, "org", capturedDTO.RepoOwner)
	assert.Equal(t, "deadbeef", capturedDTO.GitSHA)
	assert.Equal(t, "tenant1-ue1-prod", capturedDTO.Stack)
	assert.Equal(t, "vpc", capturedDTO.Component)

	// The SARIF payload must be a valid SARIF 2.1.0 doc with one result.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(capturedDTO.SARIF, &decoded))
	assert.Equal(t, "2.1.0", decoded["version"])
}

func TestUploadReportToAtmosPro_ClientCreationFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := git.NewMockGitRepoInterface(ctrl)
	repo.EXPECT().GetLocalRepoInfo().Return(&git.RepoInfo{RepoUrl: "https://github.com/org/repo"}, nil)
	repo.EXPECT().GetCurrentCommitSHA().Return("", nil)

	withFactories(t, repo, nil, errors.New("no atmos pro token"))

	err := uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAWSSecurityUploadFailed))
}

func TestUploadReportToAtmosPro_ServerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := git.NewMockGitRepoInterface(ctrl)
	repo.EXPECT().GetLocalRepoInfo().Return(&git.RepoInfo{RepoUrl: "https://github.com/org/repo"}, nil)
	repo.EXPECT().GetCurrentCommitSHA().Return("", nil)

	client := pro.NewMockAPIClient(ctrl)
	client.EXPECT().UploadSecurityFindings(gomock.Any()).Return(errors.New("server rejected"))

	withFactories(t, repo, client, nil)

	err := uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", "")
	require.Error(t, err)
}

func TestUploadReportToAtmosPro_RepoInfoFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := git.NewMockGitRepoInterface(ctrl)
	repo.EXPECT().GetLocalRepoInfo().Return(nil, errors.New("not a git repository"))

	// Client must not be invoked when repo info fails — gomock will fail the
	// test if UploadSecurityFindings is called without an expectation.
	client := pro.NewMockAPIClient(ctrl)

	withFactories(t, repo, client, nil)

	err := uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAWSSecurityUploadFailed))
}

func TestUploadReportToAtmosPro_MissingShaIsNonFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := git.NewMockGitRepoInterface(ctrl)
	repo.EXPECT().GetLocalRepoInfo().Return(&git.RepoInfo{RepoUrl: "https://github.com/org/repo"}, nil)
	repo.EXPECT().GetCurrentCommitSHA().Return("", errors.New("detached HEAD"))

	client := pro.NewMockAPIClient(ctrl)
	var capturedDTO *dtos.SecurityFindingsUploadRequest
	client.EXPECT().UploadSecurityFindings(gomock.Any()).
		DoAndReturn(func(dto *dtos.SecurityFindingsUploadRequest) error {
			capturedDTO = dto
			return nil
		})

	withFactories(t, repo, client, nil)

	require.NoError(t, uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", ""))
	require.NotNil(t, capturedDTO)
	assert.Empty(t, capturedDTO.GitSHA)
}
