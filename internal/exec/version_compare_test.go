package exec

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

// versionComparisonCases covers semantic ordering: only a strictly newer, parseable release is suggested.
var versionComparisonCases = []struct {
	name, current, latest, want string
	ok                          bool
}{
	{"newer patch release", "1.234.0", "v1.234.1", "1.234.1", true},
	{"newer minor with v prefixes", "v1.9.0", "v1.10.0", "1.10.0", true},
	{"release after its prerelease", "1.235.0-rc.1", "v1.235.0", "1.235.0", true},
	{"same version", "v1.234.1", "1.234.1", "", false},
	{"candidate build newer than release", "1.235.0-candidate", "v1.234.1", "", false},
	{"nightly build newer than release", "1.235.0-nightly.44967635975", "v1.234.1", "", false},
	{"older release", "1.235.0", "v1.234.1", "", false},
	{"lexically larger but older release", "1.10.0", "v1.9.0", "", false},
	{"unparsable current version", "test", "v1.234.1", "", false},
	{"unparsable latest release", "1.234.1", "latest", "", false},
}

func TestGetLatestVersionComparesSemantically(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })
	for _, tc := range versionComparisonCases {
		t.Run(tc.name, func(t *testing.T) {
			version.Version = tc.current
			mockExec := NewMockVersionExecutor(gomock.NewController(t))
			mockExec.EXPECT().GetLatestGitHubRepoRelease().Return(tc.latest, nil)
			v := versionExec{atmosConfig: &schema.AtmosConfiguration{}, getLatestGitHubRepoRelease: mockExec.GetLatestGitHubRepoRelease}

			got, ok := v.GetLatestVersion(true)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCheckReleaseNeverSuggestsOlderRelease(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })
	for _, tc := range versionComparisonCases {
		t.Run(tc.name, func(t *testing.T) {
			version.Version = tc.current
			mockExec := NewMockVersionExecutor(gomock.NewController(t))
			mockExec.EXPECT().GetLatestGitHubRepoRelease().Return(tc.latest, nil)
			if tc.ok {
				mockExec.EXPECT().PrintMessageToUpgradeToAtmosLatestRelease(tc.want).Times(1)
			}
			v := versionExec{
				atmosConfig:                               &schema.AtmosConfiguration{},
				getLatestGitHubRepoRelease:                mockExec.GetLatestGitHubRepoRelease,
				printMessageToUpgradeToAtmosLatestRelease: mockExec.PrintMessageToUpgradeToAtmosLatestRelease,
			}
			v.checkRelease()
		})
	}
}

func TestDisplayVersionInFormatOmitsOlderUpdateVersion(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })
	version.Version = "1.235.0-candidate"
	mockExec := NewMockVersionExecutor(gomock.NewController(t))
	mockExec.EXPECT().GetLatestGitHubRepoRelease().Return("v1.234.1", nil)
	v := versionExec{atmosConfig: &schema.AtmosConfiguration{}, getLatestGitHubRepoRelease: mockExec.GetLatestGitHubRepoRelease}

	var stdout, stderr bytes.Buffer
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&vendorModelTestStreams{stdout: &stdout, stderr: &stderr}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(func() { data.InitWriter(iolib.GetContext()) })

	require.NoError(t, v.displayVersionInFormat(true, "json"))
	assert.Contains(t, stdout.String(), `"1.235.0-candidate"`)
	assert.NotContains(t, stdout.String(), "update_version")
}
