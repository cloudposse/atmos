package security

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgsecurity "github.com/cloudposse/atmos/pkg/aws/security"
	"github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeGitRepo implements git.GitRepoInterface for upload tests.
type fakeGitRepo struct {
	info    *git.RepoInfo
	infoErr error
	sha     string
	shaErr  error
}

func (f *fakeGitRepo) GetLocalRepoInfo() (*git.RepoInfo, error) {
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return f.info, nil
}

func (f *fakeGitRepo) GetRepoInfo(_ *gogit.Repository) (git.RepoInfo, error) {
	if f.info == nil {
		return git.RepoInfo{}, nil
	}
	return *f.info, nil
}

func (f *fakeGitRepo) GetCurrentCommitSHA() (string, error) {
	return f.sha, f.shaErr
}

// fakeProClient implements pro.AtmosProAPIClientInterface for upload tests.
// Only UploadSecurityFindings is exercised; the rest panic so accidental calls
// are loud.
type fakeProClient struct {
	called    bool
	lastDTO   *dtos.SecurityFindingsUploadRequest
	uploadErr error
}

func (f *fakeProClient) UploadInstances(*dtos.InstancesUploadRequest) error {
	panic("UploadInstances not expected in upload tests")
}

func (f *fakeProClient) UploadInstanceStatus(*dtos.InstanceStatusUploadRequest) error {
	panic("UploadInstanceStatus not expected in upload tests")
}

func (f *fakeProClient) UploadAffectedStacks(*dtos.UploadAffectedStacksRequest) error {
	panic("UploadAffectedStacks not expected in upload tests")
}

func (f *fakeProClient) UploadSecurityFindings(dto *dtos.SecurityFindingsUploadRequest) error {
	f.called = true
	f.lastDTO = dto
	return f.uploadErr
}

func (f *fakeProClient) LockStack(*dtos.LockStackRequest) (dtos.LockStackResponse, error) {
	panic("LockStack not expected in upload tests")
}

func (f *fakeProClient) UnlockStack(*dtos.UnlockStackRequest) (dtos.UnlockStackResponse, error) {
	panic("UnlockStack not expected in upload tests")
}

// withFactories swaps the package-level factories for the duration of the test.
func withFactories(t *testing.T, fakeRepo git.GitRepoInterface, fakeClient pro.AtmosProAPIClientInterface, clientErr error) {
	t.Helper()
	origRepo := gitRepoFactory
	origClient := proClientFactory
	gitRepoFactory = func() git.GitRepoInterface { return fakeRepo }
	proClientFactory = func(*schema.AtmosConfiguration) (pro.AtmosProAPIClientInterface, error) {
		if clientErr != nil {
			return nil, clientErr
		}
		return fakeClient, nil
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
	repo := &fakeGitRepo{
		info: &git.RepoInfo{
			RepoUrl:   "https://github.com/org/repo",
			RepoName:  "repo",
			RepoOwner: "org",
			RepoHost:  "github.com",
		},
		sha: "deadbeef",
	}
	client := &fakeProClient{}
	withFactories(t, repo, client, nil)

	cfg := &schema.AtmosConfiguration{}
	report := newUploadTestReport()

	require.NoError(t, uploadReportToAtmosPro(cfg, report, "tenant1-ue1-prod", "vpc"))
	require.True(t, client.called, "expected UploadSecurityFindings to be called")
	require.NotNil(t, client.lastDTO)
	assert.Equal(t, "sarif", client.lastDTO.Format)
	assert.Equal(t, "org", client.lastDTO.RepoOwner)
	assert.Equal(t, "deadbeef", client.lastDTO.GitSHA)
	assert.Equal(t, "tenant1-ue1-prod", client.lastDTO.Stack)
	assert.Equal(t, "vpc", client.lastDTO.Component)

	// The SARIF payload must be a valid SARIF 2.1.0 doc with one result.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(client.lastDTO.SARIF, &decoded))
	assert.Equal(t, "2.1.0", decoded["version"])
}

func TestUploadReportToAtmosPro_ClientCreationFails(t *testing.T) {
	repo := &fakeGitRepo{
		info: &git.RepoInfo{RepoUrl: "https://github.com/org/repo"},
	}
	withFactories(t, repo, nil, errors.New("no atmos pro token"))

	err := uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAWSSecurityUploadFailed))
}

func TestUploadReportToAtmosPro_ServerError(t *testing.T) {
	repo := &fakeGitRepo{
		info: &git.RepoInfo{RepoUrl: "https://github.com/org/repo"},
	}
	client := &fakeProClient{uploadErr: errors.New("server rejected")}
	withFactories(t, repo, client, nil)

	err := uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", "")
	require.Error(t, err)
}

func TestUploadReportToAtmosPro_RepoInfoFails(t *testing.T) {
	repo := &fakeGitRepo{infoErr: errors.New("not a git repository")}
	client := &fakeProClient{}
	withFactories(t, repo, client, nil)

	err := uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAWSSecurityUploadFailed))
	assert.False(t, client.called, "client must not be invoked when repo info fails")
}

func TestUploadReportToAtmosPro_MissingShaIsNonFatal(t *testing.T) {
	repo := &fakeGitRepo{
		info:   &git.RepoInfo{RepoUrl: "https://github.com/org/repo"},
		shaErr: errors.New("detached HEAD"),
	}
	client := &fakeProClient{}
	withFactories(t, repo, client, nil)

	require.NoError(t, uploadReportToAtmosPro(&schema.AtmosConfiguration{}, newUploadTestReport(), "", ""))
	require.True(t, client.called)
	assert.Empty(t, client.lastDTO.GitSHA)
}
