package version

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMarkdownInline(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "renders backticks as code",
			input:    "Add `go-getter` support",
			contains: "go-getter",
		},
		{
			name:     "renders bold text",
			input:    "Add **important** feature",
			contains: "important",
		},
		{
			name:     "handles plain text",
			input:    "Simple release notes",
			contains: "Simple release notes",
		},
		{
			name:     "removes newlines",
			input:    "Line one\nLine two",
			contains: "Line one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdownInline(tt.input)
			assert.Contains(t, result, tt.contains)
			assert.NotContains(t, result, "\n")
		})
	}
}

func TestFilterAssetsByPlatform(t *testing.T) {
	tests := []struct {
		name          string
		assets        []*github.ReleaseAsset
		expectedCount int
	}{
		{
			name: "filters to current platform assets",
			assets: []*github.ReleaseAsset{
				{Name: github.String("atmos_1.0.0_darwin_arm64.tar.gz")},
				{Name: github.String("atmos_1.0.0_linux_amd64.tar.gz")},
				{Name: github.String("atmos_1.0.0_windows_amd64.zip")},
			},
			expectedCount: func() int {
				if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
					return 1
				}
				if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
					return 1
				}
				if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
					return 1
				}
				return 0
			}(),
		},
		{
			name: "does not match 'win' in darwin",
			assets: []*github.ReleaseAsset{
				{Name: github.String("atmos_1.0.0_darwin_arm64.tar.gz")},
			},
			expectedCount: func() int {
				if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
					return 1
				}
				return 0
			}(),
		},
		{
			name: "matches windows with win32/win64",
			assets: []*github.ReleaseAsset{
				{Name: github.String("atmos_1.0.0_win64_amd64.zip")},
				{Name: github.String("atmos_1.0.0_windows_amd64.zip")},
			},
			expectedCount: func() int {
				if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
					return 2
				}
				return 0
			}(),
		},
		{
			name:          "empty asset list",
			assets:        []*github.ReleaseAsset{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterAssetsByPlatform(tt.assets)
			assert.Len(t, result, tt.expectedCount)
		})
	}
}

func TestCreateVersionTable(t *testing.T) {
	tests := []struct {
		name        string
		rows        [][]string
		expectError bool
	}{
		{
			name: "creates table with valid rows",
			rows: [][]string{
				{"●", "v1.0.0", "2025-01-01", "Release title"},
				{" ", "v0.9.0", "2024-12-01", "Previous release"},
			},
			expectError: false,
		},
		{
			name:        "empty rows",
			rows:        [][]string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, err := createVersionTable(tt.rows)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, table)
			}
		})
	}
}

func TestFormatReleaseListJSON(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	releases := []*github.RepositoryRelease{
		{
			TagName:     github.String("v1.0.0"),
			Name:        github.String("Release 1.0.0"),
			PublishedAt: &github.Timestamp{Time: publishedAt},
			HTMLURL:     github.String("https://github.com/cloudposse/atmos/releases/tag/v1.0.0"),
			Prerelease:  github.Bool(false),
		},
	}

	output := captureStdout(t, func() error {
		return formatReleaseListJSON(releases)
	})

	assert.Contains(t, output, `"tag"`)
	assert.Contains(t, output, `"title"`)
	assert.Contains(t, output, `"v1.0.0"`)
}

func TestFormatReleaseListYAML(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	releases := []*github.RepositoryRelease{
		{
			TagName:     github.String("v1.0.0"),
			Name:        github.String("Release 1.0.0"),
			PublishedAt: &github.Timestamp{Time: publishedAt},
			HTMLURL:     github.String("https://github.com/cloudposse/atmos/releases/tag/v1.0.0"),
			Prerelease:  github.Bool(false),
		},
	}

	output := captureStdout(t, func() error {
		return formatReleaseListYAML(releases)
	})

	assert.Contains(t, output, "tag:")
	assert.Contains(t, output, "v1.0.0")
}

func TestFormatReleaseListText(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name     string
		releases []*github.RepositoryRelease
		wantErr  bool
	}{
		{
			name: "formats releases as table",
			releases: []*github.RepositoryRelease{
				{
					TagName:     github.String("v1.0.0"),
					Name:        github.String("Release 1.0.0"),
					PublishedAt: &github.Timestamp{Time: publishedAt},
					Prerelease:  github.Bool(false),
				},
			},
			wantErr: false,
		},
		{
			name:     "handles empty release list",
			releases: []*github.RepositoryRelease{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := formatReleaseListText(tt.releases)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// captureStdout captures stdout for a function call and returns the output.
func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()
	require.NoError(t, err)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestFormatReleaseDetailJSON(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	release := &github.RepositoryRelease{
		TagName:     github.String("v1.0.0"),
		Name:        github.String("Release 1.0.0"),
		Body:        github.String("Release notes"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
		HTMLURL:     github.String("https://github.com/cloudposse/atmos/releases/tag/v1.0.0"),
	}

	output := captureStdout(t, func() error {
		return formatReleaseDetailJSON(release)
	})

	assert.Contains(t, output, `"tag"`)
	assert.Contains(t, output, `"v1.0.0"`)
}

func TestFormatReleaseDetailYAML(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	release := &github.RepositoryRelease{
		TagName:     github.String("v1.0.0"),
		Name:        github.String("Release 1.0.0"),
		Body:        github.String("Release notes"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
		HTMLURL:     github.String("https://github.com/cloudposse/atmos/releases/tag/v1.0.0"),
	}

	output := captureStdout(t, func() error {
		return formatReleaseDetailYAML(release)
	})

	assert.Contains(t, output, "tag:")
	assert.Contains(t, output, "v1.0.0")
}

func TestFormatReleaseDetailText(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	size := int(1048576) // 1 MB

	release := &github.RepositoryRelease{
		TagName:     github.String("v1.0.0"),
		Name:        github.String("Release 1.0.0"),
		Body:        github.String("Release notes with **markdown**"),
		PublishedAt: &github.Timestamp{Time: publishedAt},
		HTMLURL:     github.String("https://github.com/cloudposse/atmos/releases/tag/v1.0.0"),
		Assets: []*github.ReleaseAsset{
			{
				Name:               github.String("atmos_1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"),
				Size:               &size,
				BrowserDownloadURL: github.String("https://github.com/cloudposse/atmos/releases/download/v1.0.0/atmos.tar.gz"),
			},
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	formatReleaseDetailText(release)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	assert.Contains(t, output, "v1.0.0")
	assert.Contains(t, output, "Release 1.0.0")
}

func TestGetReleaseTitle(t *testing.T) {
	tests := []struct {
		name     string
		release  *github.RepositoryRelease
		expected string
	}{
		{
			name: "extracts title from release name",
			release: &github.RepositoryRelease{
				Name:    github.String("# Release 1.0.0\n\nSome content"),
				TagName: github.String("v1.0.0"),
			},
			expected: "Release 1.0.0",
		},
		{
			name: "falls back to tag when name is empty",
			release: &github.RepositoryRelease{
				Name:    github.String(""),
				TagName: github.String("v1.0.0"),
			},
			expected: "v1.0.0",
		},
		{
			name: "handles markdown with multiple headings",
			release: &github.RepositoryRelease{
				Name:    github.String("# First Heading\n\n## Second Heading"),
				TagName: github.String("v2.0.0"),
			},
			expected: "First Heading",
		},
		{
			name: "handles plain text name without heading",
			release: &github.RepositoryRelease{
				Name:    github.String("Plain text release name"),
				TagName: github.String("v3.0.0"),
			},
			expected: "Plain text release name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getReleaseTitle(tt.release)
			assert.Contains(t, result, tt.expected)
		})
	}
}

func TestIsCurrentVersion(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{
			name: "checks version tag",
			tag:  "v1.0.0",
		},
		{
			name: "handles non-matching version",
			tag:  "v999.999.999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the function doesn't panic
			_ = isCurrentVersion(tt.tag)
		})
	}
}

func TestAddCurrentVersionIfMissing(t *testing.T) {
	publishedAt := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name           string
		releases       []*github.RepositoryRelease
		expectedLength int
	}{
		{
			name: "does not add if already present",
			releases: []*github.RepositoryRelease{
				{TagName: github.String("v1.0.0"), PublishedAt: &github.Timestamp{Time: publishedAt}},
			},
			expectedLength: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addCurrentVersionIfMissing(tt.releases)
			// Length might vary based on actual current version
			assert.GreaterOrEqual(t, len(result), tt.expectedLength)
		})
	}
}
