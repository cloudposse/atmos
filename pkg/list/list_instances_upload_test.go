package list

import (
	"errors"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgCfg "github.com/cloudposse/atmos/pkg/config"
	pkgGit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestUploadInstancesWithDeps_Success tests the happy path where all operations succeed.
func TestUploadInstancesWithDeps_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mocks
	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)
	mockAPIClient := pro.NewMockAPIClient(ctrl)

	// Test data
	instances := []schema.Instance{
		{Component: "vpc", Stack: "dev"},
		{Component: "eks", Stack: "dev"},
	}

	mockRepo := &git.Repository{}
	repoInfo := pkgGit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}
	atmosConfig := schema.AtmosConfiguration{}

	// Expectations
	mockGit.EXPECT().GetLocalRepo().Return(mockRepo, nil)
	mockGit.EXPECT().GetRepoInfo(mockRepo).Return(repoInfo, nil)
	mockConfig.EXPECT().InitCliConfig(gomock.Any(), false).Return(atmosConfig, nil)
	mockClientFactory.EXPECT().NewClient(&atmosConfig).Return(mockAPIClient, nil)
	mockAPIClient.EXPECT().UploadInstances(gomock.Any()).
		Do(func(req *dtos.InstancesUploadRequest) {
			assert.Equal(t, "https://github.com/test/repo", req.RepoURL)
			assert.Equal(t, "repo", req.RepoName)
			assert.Equal(t, "test", req.RepoOwner)
			assert.Equal(t, "github.com", req.RepoHost)
			assert.Equal(t, 2, len(req.Instances))
		}).
		Return(nil)

	// Execute
	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	// Verify
	assert.NoError(t, err)
}

// TestUploadInstancesWithDeps_GitRepoError tests error handling when git repo cannot be opened.
func TestUploadInstancesWithDeps_GitRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)

	instances := []schema.Instance{{Component: "vpc", Stack: "dev"}}
	expectedErr := errors.New("not a git repository")

	mockGit.EXPECT().GetLocalRepo().Return(nil, expectedErr)

	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToGetLocalRepo))
	assert.ErrorContains(t, err, "not a git repository")
}

// TestUploadInstancesWithDeps_GitInfoError tests error handling when repo info cannot be retrieved.
func TestUploadInstancesWithDeps_GitInfoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)

	instances := []schema.Instance{{Component: "vpc", Stack: "dev"}}
	mockRepo := &git.Repository{}
	expectedErr := errors.New("no remote configured")

	mockGit.EXPECT().GetLocalRepo().Return(mockRepo, nil)
	mockGit.EXPECT().GetRepoInfo(mockRepo).Return(pkgGit.RepoInfo{}, expectedErr)

	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToGetRepoInfo))
	assert.ErrorContains(t, err, "no remote configured")
}

// TestUploadInstancesWithDeps_ConfigError tests error handling when config initialization fails.
func TestUploadInstancesWithDeps_ConfigError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)

	instances := []schema.Instance{{Component: "vpc", Stack: "dev"}}
	mockRepo := &git.Repository{}
	repoInfo := pkgGit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}
	expectedErr := errors.New("config file not found")

	mockGit.EXPECT().GetLocalRepo().Return(mockRepo, nil)
	mockGit.EXPECT().GetRepoInfo(mockRepo).Return(repoInfo, nil)
	mockConfig.EXPECT().InitCliConfig(gomock.Any(), false).
		Return(schema.AtmosConfiguration{}, expectedErr)

	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToInitConfig))
	assert.ErrorContains(t, err, "config file not found")
}

// TestUploadInstancesWithDeps_APIClientError tests error handling when API client creation fails.
func TestUploadInstancesWithDeps_APIClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)

	instances := []schema.Instance{{Component: "vpc", Stack: "dev"}}
	mockRepo := &git.Repository{}
	repoInfo := pkgGit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}
	atmosConfig := schema.AtmosConfiguration{}
	expectedErr := errors.New("missing API token")

	mockGit.EXPECT().GetLocalRepo().Return(mockRepo, nil)
	mockGit.EXPECT().GetRepoInfo(mockRepo).Return(repoInfo, nil)
	mockConfig.EXPECT().InitCliConfig(gomock.Any(), false).Return(atmosConfig, nil)
	mockClientFactory.EXPECT().NewClient(&atmosConfig).Return(nil, expectedErr)

	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToCreateAPIClient))
	assert.ErrorContains(t, err, "missing API token")
}

// TestUploadInstancesWithDeps_UploadError tests error handling when the upload operation fails.
func TestUploadInstancesWithDeps_UploadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)
	mockAPIClient := pro.NewMockAPIClient(ctrl)

	instances := []schema.Instance{{Component: "vpc", Stack: "dev"}}
	mockRepo := &git.Repository{}
	repoInfo := pkgGit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}
	atmosConfig := schema.AtmosConfiguration{}
	expectedErr := errors.New("API server unreachable")

	mockGit.EXPECT().GetLocalRepo().Return(mockRepo, nil)
	mockGit.EXPECT().GetRepoInfo(mockRepo).Return(repoInfo, nil)
	mockConfig.EXPECT().InitCliConfig(gomock.Any(), false).Return(atmosConfig, nil)
	mockClientFactory.EXPECT().NewClient(&atmosConfig).Return(mockAPIClient, nil)
	mockAPIClient.EXPECT().UploadInstances(gomock.Any()).Return(expectedErr)

	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToUploadInstances))
	assert.ErrorContains(t, err, "API server unreachable")
}

// TestUploadInstancesWithDeps_IncompleteRepoInfo tests handling of incomplete repository information.
// This should log a warning but still proceed with the upload.
func TestUploadInstancesWithDeps_IncompleteRepoInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)
	mockAPIClient := pro.NewMockAPIClient(ctrl)

	instances := []schema.Instance{{Component: "vpc", Stack: "dev"}}
	mockRepo := &git.Repository{}
	// Incomplete repo info - missing RepoOwner
	repoInfo := pkgGit.RepoInfo{
		RepoUrl:  "https://github.com/test/repo",
		RepoName: "repo",
		RepoHost: "github.com",
		// RepoOwner is empty
	}
	atmosConfig := schema.AtmosConfiguration{}

	mockGit.EXPECT().GetLocalRepo().Return(mockRepo, nil)
	mockGit.EXPECT().GetRepoInfo(mockRepo).Return(repoInfo, nil)
	mockConfig.EXPECT().InitCliConfig(gomock.Any(), false).Return(atmosConfig, nil)
	mockClientFactory.EXPECT().NewClient(&atmosConfig).Return(mockAPIClient, nil)
	mockAPIClient.EXPECT().UploadInstances(gomock.Any()).Return(nil)

	// Should succeed despite incomplete info (warning is logged)
	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	assert.NoError(t, err)
}

// TestUploadInstancesWithDeps_EmptyInstances tests behavior with empty instance list.
func TestUploadInstancesWithDeps_EmptyInstances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGit := pkgGit.NewMockRepositoryOperations(ctrl)
	mockConfig := pkgCfg.NewMockLoader(ctrl)
	mockClientFactory := pro.NewMockClientFactory(ctrl)
	mockAPIClient := pro.NewMockAPIClient(ctrl)

	instances := []schema.Instance{} // Empty list
	mockRepo := &git.Repository{}
	repoInfo := pkgGit.RepoInfo{
		RepoUrl:   "https://github.com/test/repo",
		RepoName:  "repo",
		RepoOwner: "test",
		RepoHost:  "github.com",
	}
	atmosConfig := schema.AtmosConfiguration{}

	mockGit.EXPECT().GetLocalRepo().Return(mockRepo, nil)
	mockGit.EXPECT().GetRepoInfo(mockRepo).Return(repoInfo, nil)
	mockConfig.EXPECT().InitCliConfig(gomock.Any(), false).Return(atmosConfig, nil)
	mockClientFactory.EXPECT().NewClient(&atmosConfig).Return(mockAPIClient, nil)
	mockAPIClient.EXPECT().UploadInstances(gomock.Any()).
		Do(func(req *dtos.InstancesUploadRequest) {
			assert.Equal(t, 0, len(req.Instances))
		}).
		Return(nil)

	err := uploadInstancesWithDeps(instances, mockGit, mockConfig, mockClientFactory)

	assert.NoError(t, err)
}
