package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetArtifactNameForPlatform(t *testing.T) {
	// This test verifies the platform mapping logic.
	// The actual result depends on the runtime platform.

	artifactName, err := getArtifactNameForPlatform()

	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH == "amd64" {
			assert.NoError(t, err)
			assert.Equal(t, "build-artifacts-linux", artifactName)
		} else {
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrNoArtifactForPlatform)
		}
	case "darwin":
		if runtime.GOARCH == "arm64" {
			assert.NoError(t, err)
			assert.Equal(t, "build-artifacts-macos", artifactName)
		} else {
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrNoArtifactForPlatform)
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			assert.NoError(t, err)
			assert.Equal(t, "build-artifacts-windows", artifactName)
		} else {
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrNoArtifactForPlatform)
		}
	default:
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedPlatform)
	}
}

func TestSupportedPRPlatforms(t *testing.T) {
	platforms := SupportedPRPlatforms()

	assert.Len(t, platforms, 3)
	assert.Contains(t, platforms, "linux/amd64")
	assert.Contains(t, platforms, "darwin/arm64")
	assert.Contains(t, platforms, "windows/amd64")
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrPRNotFound",
			err:      ErrPRNotFound,
			expected: true,
		},
		{
			name:     "wrapped ErrPRNotFound",
			err:      fmt.Errorf("context: %w", ErrPRNotFound),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNotFoundError(tt.err))
		})
	}
}

func TestIsNoWorkflowError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrNoWorkflowRunFound",
			err:      ErrNoWorkflowRunFound,
			expected: true,
		},
		{
			name:     "wrapped ErrNoWorkflowRunFound",
			err:      fmt.Errorf("context: %w", ErrNoWorkflowRunFound),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNoWorkflowError(tt.err))
		})
	}
}

func TestIsNoArtifactError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrNoArtifactFound",
			err:      ErrNoArtifactFound,
			expected: true,
		},
		{
			name:     "wrapped ErrNoArtifactFound",
			err:      fmt.Errorf("context: %w", ErrNoArtifactFound),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNoArtifactError(tt.err))
		})
	}
}

func TestIsPlatformError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrNoArtifactForPlatform",
			err:      ErrNoArtifactForPlatform,
			expected: true,
		},
		{
			name:     "wrapped ErrNoArtifactForPlatform",
			err:      fmt.Errorf("context: %w", ErrNoArtifactForPlatform),
			expected: true,
		},
		{
			name:     "direct ErrUnsupportedPlatform",
			err:      ErrUnsupportedPlatform,
			expected: true,
		},
		{
			name:     "wrapped ErrUnsupportedPlatform",
			err:      fmt.Errorf("context: %w", ErrUnsupportedPlatform),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsPlatformError(tt.err))
		})
	}
}

// --- Mock-based unit tests for getPRHeadSHA ---

func TestArtifactFetcherGetPRHeadSHA_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)

	sha := "abc123def456"
	pr := &github.PullRequest{
		Head: &github.PullRequestBranch{
			SHA: &sha,
		},
	}

	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 42).
		Return(pr, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, nil).GetPRHeadSHA(ctx, "owner", "repo", 42)

	assert.NoError(t, err)
	assert.Equal(t, "abc123def456", result)
}

func TestArtifactFetcherGetPRHeadSHA_PRNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)

	resp := &github.Response{
		Response: &http.Response{StatusCode: 404},
	}

	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 999).
		Return(nil, resp, errors.New("not found"))

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, nil).GetPRHeadSHA(ctx, "owner", "repo", 999)

	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, ErrPRNotFound)
}

func TestArtifactFetcherGetPRHeadSHA_NilHead(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)

	pr := &github.PullRequest{
		Head: nil,
	}

	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 42).
		Return(pr, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, nil).GetPRHeadSHA(ctx, "owner", "repo", 42)

	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, ErrPRNotFound)
}

func TestArtifactFetcherGetPRHeadSHA_NilHeadSHA(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)

	pr := &github.PullRequest{
		Head: &github.PullRequestBranch{
			SHA: nil,
		},
	}

	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 42).
		Return(pr, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, nil).GetPRHeadSHA(ctx, "owner", "repo", 42)

	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, ErrPRNotFound)
}

func TestArtifactFetcherGetPRHeadSHA_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)

	resp := &github.Response{
		Response: &http.Response{StatusCode: 500},
	}

	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 42).
		Return(nil, resp, errors.New("internal server error"))

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, nil).GetPRHeadSHA(ctx, "owner", "repo", 42)

	assert.Error(t, err)
	assert.Empty(t, result)
}

// --- Mock-based unit tests for findSuccessfulWorkflowRun ---

func TestFindSuccessfulWorkflowRun_MatchesCorrectWorkflow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	runStartedAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	runID := int64(12345)
	workflowRunName := "Tests"
	conclusion := "success"
	otherName := "Build"

	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{
			{
				Name:       &otherName,
				Conclusion: &conclusion,
				ID:         github.Int64(99999),
			},
			{
				Name:         &workflowRunName,
				Conclusion:   &conclusion,
				ID:           &runID,
				RunStartedAt: &github.Timestamp{Time: runStartedAt},
			},
		},
	}

	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	ctx := context.Background()
	result, err := findSuccessfulWorkflowRun(ctx, mockActions, "owner", "repo", "abc123")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(12345), result.ID)
	assert.Equal(t, runStartedAt, result.RunStartedAt)
}

func TestFindSuccessfulWorkflowRun_IgnoresNonSuccessConclusion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	workflowRunName := "Tests"
	failure := "failure"

	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{
			{
				Name:       &workflowRunName,
				Conclusion: &failure,
				ID:         github.Int64(12345),
			},
		},
	}

	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	ctx := context.Background()
	result, err := findSuccessfulWorkflowRun(ctx, mockActions, "owner", "repo", "abc123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoWorkflowRunFound)
}

func TestFindSuccessfulWorkflowRun_NoMatchingRuns(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{},
	}

	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	ctx := context.Background()
	result, err := findSuccessfulWorkflowRun(ctx, mockActions, "owner", "repo", "abc123")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoWorkflowRunFound)
}

func TestFindSuccessfulWorkflowRun_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(nil, nil, errors.New("API error"))

	ctx := context.Background()
	result, err := findSuccessfulWorkflowRun(ctx, mockActions, "owner", "repo", "abc123")

	assert.Error(t, err)
	assert.Nil(t, result)
}

// --- Mock-based unit tests for findArtifactByName ---

func TestFindArtifactByName_MatchesCorrectArtifact(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	targetName := "build-artifacts-macos"
	otherName := "build-artifacts-linux"
	targetID := int64(555)
	targetSize := int64(1024)
	downloadURL := "https://api.github.com/repos/owner/repo/actions/artifacts/555/zip"

	artifactList := &github.ArtifactList{
		Artifacts: []*github.Artifact{
			{
				Name:               &otherName,
				ID:                 github.Int64(444),
				SizeInBytes:        github.Int64(2048),
				ArchiveDownloadURL: github.String("https://example.com/other"),
			},
			{
				Name:               &targetName,
				ID:                 &targetID,
				SizeInBytes:        &targetSize,
				ArchiveDownloadURL: &downloadURL,
			},
		},
	}

	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(artifactList, nil, nil)

	ctx := context.Background()
	result, err := findArtifactByName(ctx, mockActions, "owner", "repo", 12345, "build-artifacts-macos")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "build-artifacts-macos", result.GetName())
	assert.Equal(t, int64(555), result.GetID())
	assert.Equal(t, int64(1024), result.GetSizeInBytes())
}

func TestFindArtifactByName_NoMatchingArtifact(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	linuxName := "build-artifacts-linux"
	windowsName := "build-artifacts-windows"

	artifactList := &github.ArtifactList{
		Artifacts: []*github.Artifact{
			{Name: &linuxName},
			{Name: &windowsName},
		},
	}

	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(artifactList, nil, nil)

	ctx := context.Background()
	result, err := findArtifactByName(ctx, mockActions, "owner", "repo", 12345, "build-artifacts-macos")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoArtifactFound)
}

func TestFindArtifactByName_EmptyArtifactList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	artifactList := &github.ArtifactList{
		Artifacts: []*github.Artifact{},
	}

	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(artifactList, nil, nil)

	ctx := context.Background()
	result, err := findArtifactByName(ctx, mockActions, "owner", "repo", 12345, "build-artifacts-macos")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoArtifactFound)
}

func TestFindArtifactByName_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(nil, nil, errors.New("API error"))

	ctx := context.Background()
	result, err := findArtifactByName(ctx, mockActions, "owner", "repo", 12345, "build-artifacts-macos")

	assert.Error(t, err)
	assert.Nil(t, result)
}

// --- Mock-based unit tests for ArtifactFetcher.GetArtifactDownloadURL ---

func TestArtifactFetcherGetArtifactDownloadURL_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	downloadURL := "https://api.github.com/repos/owner/repo/actions/artifacts/555/zip"
	artifact := &github.Artifact{
		ArchiveDownloadURL: &downloadURL,
	}

	mockActions.EXPECT().
		GetArtifact(gomock.Any(), "owner", "repo", int64(555)).
		Return(artifact, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(nil, mockActions).GetArtifactDownloadURL(ctx, "owner", "repo", 555)

	assert.NoError(t, err)
	assert.Equal(t, "https://api.github.com/repos/owner/repo/actions/artifacts/555/zip", result)
}

func TestArtifactFetcherGetArtifactDownloadURL_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	mockActions.EXPECT().
		GetArtifact(gomock.Any(), "owner", "repo", int64(555)).
		Return(nil, nil, errors.New("API error"))

	ctx := context.Background()
	result, err := NewArtifactFetcher(nil, mockActions).GetArtifactDownloadURL(ctx, "owner", "repo", 555)

	assert.Error(t, err)
	assert.Empty(t, result)
}

// --- Mock-based unit tests for ArtifactFetcher.GetPRArtifactInfo ---

// skipIfUnsupportedPlatform skips the test if the current platform does not have
// a valid artifact name mapping (e.g., darwin/amd64 is not built in CI).
func skipIfUnsupportedPlatform(t *testing.T) string {
	t.Helper()
	artifactName, err := getArtifactNameForPlatform()
	if err != nil {
		t.Skipf("Skipping test: current platform %s/%s has no artifact mapping", runtime.GOOS, runtime.GOARCH)
	}
	return artifactName
}

func TestArtifactFetcherGetPRArtifactInfo_Success(t *testing.T) {
	artifactName := skipIfUnsupportedPlatform(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)
	mockActions := NewMockActionsService(ctrl)

	// Step 1: PR head SHA.
	sha := "abc123def456"
	pr := &github.PullRequest{
		Head: &github.PullRequestBranch{SHA: &sha},
	}
	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 42).
		Return(pr, nil, nil)

	// Step 2: Successful workflow run.
	runStartedAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	workflowRunName := "Tests"
	conclusion := "success"
	runID := int64(12345)

	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{
			{
				Name:         &workflowRunName,
				Conclusion:   &conclusion,
				ID:           &runID,
				RunStartedAt: &github.Timestamp{Time: runStartedAt},
			},
		},
	}
	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	// Step 3: Artifact.
	artifactID := int64(555)
	artifactSize := int64(1024)
	downloadURL := "https://api.github.com/repos/owner/repo/actions/artifacts/555/zip"

	artifactList := &github.ArtifactList{
		Artifacts: []*github.Artifact{
			{
				Name:               &artifactName,
				ID:                 &artifactID,
				SizeInBytes:        &artifactSize,
				ArchiveDownloadURL: &downloadURL,
			},
		},
	}
	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(artifactList, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, mockActions).GetPRArtifactInfo(ctx, "owner", "repo", 42)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 42, result.PRNumber)
	assert.Equal(t, "abc123def456", result.HeadSHA)
	assert.Equal(t, int64(12345), result.RunID)
	assert.Equal(t, int64(555), result.ArtifactID)
	assert.Equal(t, artifactName, result.ArtifactName)
	assert.Equal(t, int64(1024), result.SizeInBytes)
	assert.Equal(t, "https://api.github.com/repos/owner/repo/actions/artifacts/555/zip", result.DownloadURL)
	assert.Equal(t, runStartedAt, result.RunStartedAt)
}

func TestArtifactFetcherGetPRArtifactInfo_PRNotFoundError(t *testing.T) {
	skipIfUnsupportedPlatform(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)
	mockActions := NewMockActionsService(ctrl)

	resp := &github.Response{
		Response: &http.Response{StatusCode: 404},
	}
	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 999).
		Return(nil, resp, errors.New("not found"))

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, mockActions).GetPRArtifactInfo(ctx, "owner", "repo", 999)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPRNotFound)
}

func TestArtifactFetcherGetPRArtifactInfo_NoWorkflowRunError(t *testing.T) {
	skipIfUnsupportedPlatform(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)
	mockActions := NewMockActionsService(ctrl)

	// Step 1 succeeds.
	sha := "abc123def456"
	pr := &github.PullRequest{
		Head: &github.PullRequestBranch{SHA: &sha},
	}
	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 42).
		Return(pr, nil, nil)

	// Step 2 returns no matching runs.
	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{},
	}
	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, mockActions).GetPRArtifactInfo(ctx, "owner", "repo", 42)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoWorkflowRunFound)
}

func TestArtifactFetcherGetPRArtifactInfo_NoArtifactError(t *testing.T) {
	skipIfUnsupportedPlatform(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPRS := NewMockPullRequestService(ctrl)
	mockActions := NewMockActionsService(ctrl)

	// Step 1 succeeds.
	sha := "abc123def456"
	pr := &github.PullRequest{
		Head: &github.PullRequestBranch{SHA: &sha},
	}
	mockPRS.EXPECT().
		Get(gomock.Any(), "owner", "repo", 42).
		Return(pr, nil, nil)

	// Step 2 succeeds.
	workflowRunName := "Tests"
	conclusion := "success"
	runID := int64(12345)
	runStartedAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{
			{
				Name:         &workflowRunName,
				Conclusion:   &conclusion,
				ID:           &runID,
				RunStartedAt: &github.Timestamp{Time: runStartedAt},
			},
		},
	}
	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	// Step 3 returns no matching artifacts.
	artifactList := &github.ArtifactList{
		Artifacts: []*github.Artifact{},
	}
	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(artifactList, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(mockPRS, mockActions).GetPRArtifactInfo(ctx, "owner", "repo", 42)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoArtifactFound)
}

// --- Mock-based unit tests for ArtifactFetcher.GetSHAArtifactInfo ---

func TestArtifactFetcherGetSHAArtifactInfo_Success(t *testing.T) {
	artifactName := skipIfUnsupportedPlatform(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	// Step 1: Successful workflow run.
	runStartedAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	workflowRunName := "Tests"
	conclusion := "success"
	runID := int64(12345)

	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{
			{
				Name:         &workflowRunName,
				Conclusion:   &conclusion,
				ID:           &runID,
				RunStartedAt: &github.Timestamp{Time: runStartedAt},
			},
		},
	}
	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	// Step 2: Artifact.
	artifactID := int64(555)
	artifactSize := int64(2048)
	downloadURL := "https://api.github.com/repos/owner/repo/actions/artifacts/555/zip"

	artifactList := &github.ArtifactList{
		Artifacts: []*github.Artifact{
			{
				Name:               &artifactName,
				ID:                 &artifactID,
				SizeInBytes:        &artifactSize,
				ArchiveDownloadURL: &downloadURL,
			},
		},
	}
	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(artifactList, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(nil, mockActions).GetSHAArtifactInfo(ctx, "owner", "repo", "abc123def456")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "abc123def456", result.HeadSHA)
	assert.Equal(t, int64(12345), result.RunID)
	assert.Equal(t, int64(555), result.ArtifactID)
	assert.Equal(t, artifactName, result.ArtifactName)
	assert.Equal(t, int64(2048), result.SizeInBytes)
	assert.Equal(t, "https://api.github.com/repos/owner/repo/actions/artifacts/555/zip", result.DownloadURL)
	assert.Equal(t, runStartedAt, result.RunStartedAt)
}

func TestArtifactFetcherGetSHAArtifactInfo_NoWorkflowRunError(t *testing.T) {
	skipIfUnsupportedPlatform(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{},
	}
	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(nil, mockActions).GetSHAArtifactInfo(ctx, "owner", "repo", "abc123def456")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoWorkflowRunFound)
}

func TestArtifactFetcherGetSHAArtifactInfo_NoArtifactError(t *testing.T) {
	skipIfUnsupportedPlatform(t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockActions := NewMockActionsService(ctrl)

	// Step 1 succeeds.
	workflowRunName := "Tests"
	conclusion := "success"
	runID := int64(12345)
	runStartedAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	runs := &github.WorkflowRuns{
		WorkflowRuns: []*github.WorkflowRun{
			{
				Name:         &workflowRunName,
				Conclusion:   &conclusion,
				ID:           &runID,
				RunStartedAt: &github.Timestamp{Time: runStartedAt},
			},
		},
	}
	mockActions.EXPECT().
		ListRepositoryWorkflowRuns(gomock.Any(), "owner", "repo", gomock.Any()).
		Return(runs, nil, nil)

	// Step 2 returns no matching artifacts.
	artifactList := &github.ArtifactList{
		Artifacts: []*github.Artifact{},
	}
	mockActions.EXPECT().
		ListWorkflowRunArtifacts(gomock.Any(), "owner", "repo", int64(12345), gomock.Any()).
		Return(artifactList, nil, nil)

	ctx := context.Background()
	result, err := NewArtifactFetcher(nil, mockActions).GetSHAArtifactInfo(ctx, "owner", "repo", "abc123def456")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrNoArtifactFound)
}

// Note: Full integration tests for GetPRArtifactInfo require a real GitHub token
// and network access. Those would be in an integration test file with appropriate
// skip conditions.
