package list

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/profile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRenderTable tests the RenderTable function.
func TestRenderTable(t *testing.T) {
	tests := []struct {
		name             string
		profiles         []profile.ProfileInfo
		expectedContains []string
		expectedCount    int
	}{
		{
			name:     "empty profile list",
			profiles: []profile.ProfileInfo{},
			expectedContains: []string{
				"No profiles configured",
			},
			expectedCount: 0,
		},
		{
			name: "single profile",
			profiles: []profile.ProfileInfo{
				{
					Name:         "dev",
					Path:         "/path/to/dev",
					LocationType: "project",
					Files:        []string{"atmos.yaml"},
				},
			},
			expectedContains: []string{
				"PROFILES",
				"NAME",
				"LOCATION",
				"PATH",
				"FILES",
				"dev",
			},
			expectedCount: 1,
		},
		{
			name: "multiple profiles",
			profiles: []profile.ProfileInfo{
				{
					Name:         "production",
					Path:         "/path/to/production",
					LocationType: "project",
					Files:        []string{"atmos.yaml", "vpc.yaml"},
				},
				{
					Name:         "staging",
					Path:         "/path/to/staging",
					LocationType: "xdg",
					Files:        []string{"atmos.yaml"},
				},
				{
					Name:         "dev",
					Path:         "/path/to/dev",
					LocationType: "project-hidden",
					Files:        []string{"atmos.yaml", "config.yaml", "secrets.yaml"},
				},
			},
			expectedContains: []string{
				"PROFILES",
				"NAME",
				"LOCATION",
				"PATH",
				"FILES",
				// Every profile name must appear in the rendered table. This guards
				// against a table-height off-by-one that previously hid all rows
				// past the first.
				"production",
				"staging",
				"dev",
			},
			expectedCount: 3,
		},
		{
			name: "profiles with various file counts",
			profiles: []profile.ProfileInfo{
				{
					Name:         "no-files",
					Path:         "/path/to/no-files",
					LocationType: "project",
					Files:        []string{},
				},
				{
					Name:         "one-file",
					Path:         "/path/to/one-file",
					LocationType: "project",
					Files:        []string{"atmos.yaml"},
				},
				{
					Name:         "many-files",
					Path:         "/path/to/many-files",
					LocationType: "project",
					Files: []string{
						"file1.yaml", "file2.yaml", "file3.yaml",
						"file4.yaml", "file5.yaml", "file6.yaml",
						"file7.yaml", "file8.yaml", "file9.yaml",
						"file10.yaml", "file11.yaml",
					},
				},
			},
			expectedContains: []string{
				"PROFILES",
				"NAME",
				"no-files",
				"one-file",
				"many-files",
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RenderTable(tt.profiles, nil)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			// Check for expected content.
			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected,
					"Output should contain: %s", expected)
			}
		})
	}
}

// TestCreateProfilesTable tests the createProfilesTable function.
func TestCreateProfilesTable(t *testing.T) {
	tests := []struct {
		name          string
		profiles      []profile.ProfileInfo
		expectedRows  int
		validateTable func(t *testing.T, output string)
	}{
		{
			name: "basic table structure",
			profiles: []profile.ProfileInfo{
				{
					Name:         "test",
					Path:         "/path/to/test",
					LocationType: "project",
					Files:        []string{"atmos.yaml"},
				},
			},
			expectedRows: 1,
			validateTable: func(t *testing.T, output string) {
				assert.Contains(t, output, "NAME")
				assert.Contains(t, output, "LOCATION")
				assert.Contains(t, output, "PATH")
				assert.Contains(t, output, "test")
			},
		},
		{
			name: "profiles are sorted by name",
			profiles: []profile.ProfileInfo{
				{Name: "zulu", Path: "/z", LocationType: "project", Files: []string{}},
				{Name: "alpha", Path: "/a", LocationType: "project", Files: []string{}},
				{Name: "mike", Path: "/m", LocationType: "project", Files: []string{}},
			},
			expectedRows: 3,
			validateTable: func(t *testing.T, output string) {
				assert.Contains(t, output, "NAME")
				// All three profile names must be visible — not just the first.
				assert.Contains(t, output, "alpha")
				assert.Contains(t, output, "mike")
				assert.Contains(t, output, "zulu")
				// Sort order: alpha must precede mike must precede zulu.
				alphaIdx := strings.Index(output, "alpha")
				mikeIdx := strings.Index(output, "mike")
				zuluIdx := strings.Index(output, "zulu")
				assert.Less(t, alphaIdx, mikeIdx, "alpha should render before mike")
				assert.Less(t, mikeIdx, zuluIdx, "mike should render before zulu")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, err := createProfilesTable(tt.profiles, nil)

			require.NoError(t, err)
			output := table.View()

			require.NotEmpty(t, output)

			if tt.validateTable != nil {
				tt.validateTable(t, output)
			}
		})
	}
}

// TestBuildProfileRows tests the buildProfileRows function.
func TestBuildProfileRows(t *testing.T) {
	tests := []struct {
		name         string
		profiles     []profile.ProfileInfo
		validateRows func(t *testing.T, rows [][]string)
	}{
		{
			name: "single profile basic fields",
			profiles: []profile.ProfileInfo{
				{
					Name:         "dev",
					Path:         "/path/to/dev",
					LocationType: "project",
					Files:        []string{"atmos.yaml", "config.yaml"},
				},
			},
			validateRows: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 1)
				// Columns: [0]=indicator, [1]=name, [2]=location, [3]=path, [4]=files.
				assert.Equal(t, " ", rows[0][0]) // No active set → blank indicator.
				assert.Equal(t, "dev", rows[0][1])
				assert.Equal(t, "project", rows[0][2])
				assert.Contains(t, rows[0][3], "/path/to/dev")
				assert.NotEmpty(t, rows[0][4]) // File count.
			},
		},
		{
			name: "long path truncation",
			profiles: []profile.ProfileInfo{
				{
					Name:         "long",
					Path:         "/very/long/path/that/exceeds/the/maximum/width/allowed/for/display/in/the/table/column",
					LocationType: "project",
					Files:        []string{"atmos.yaml"},
				},
			},
			validateRows: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 1)
				path := rows[0][3]
				assert.LessOrEqual(t, len(path), pathWidth,
					"Path should be truncated to fit column width")
			},
		},
		{
			name: "file count display",
			profiles: []profile.ProfileInfo{
				{
					Name:         "zero",
					Path:         "/path",
					LocationType: "project",
					Files:        []string{},
				},
				{
					Name:         "one",
					Path:         "/path",
					LocationType: "project",
					Files:        []string{"file1.yaml"},
				},
				{
					Name:         "nine",
					Path:         "/path",
					LocationType: "project",
					Files: []string{
						"f1.yaml", "f2.yaml", "f3.yaml",
						"f4.yaml", "f5.yaml", "f6.yaml",
						"f7.yaml", "f8.yaml", "f9.yaml",
					},
				},
				{
					Name:         "ten-plus",
					Path:         "/path",
					LocationType: "project",
					Files: []string{
						"f1.yaml", "f2.yaml", "f3.yaml", "f4.yaml", "f5.yaml",
						"f6.yaml", "f7.yaml", "f8.yaml", "f9.yaml", "f10.yaml", "f11.yaml",
					},
				},
			},
			validateRows: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 4)

				// Sort rows by name to have predictable order.
				// Columns: [0]=indicator, [1]=name, [2]=location, [3]=path, [4]=files.
				names := []string{rows[0][1], rows[1][1], rows[2][1], rows[3][1]}

				// Find each profile by name and verify file count.
				for i, name := range names {
					switch name {
					case "zero":
						assert.Equal(t, "0", rows[i][4], "Profile with no files should show 0")
					case "one":
						// The implementation uses: string(rune('0' + len(p.Files)))
						// For 1 file: '0' + 1 = '1'
						assert.Equal(t, "1", rows[i][4], "Profile with one file should show 1")
					case "nine":
						// For 9 files: '0' + 9 = '9'
						assert.Equal(t, "9", rows[i][4], "Profile with nine files should show 9")
					case "ten-plus":
						// For 10+ files, should show "10+".
						assert.Equal(t, "10+", rows[i][4], "Profile with 11 files should show 10+")
					}
				}
			},
		},
		{
			name: "sorting by name",
			profiles: []profile.ProfileInfo{
				{Name: "charlie", Path: "/c", LocationType: "project", Files: []string{}},
				{Name: "alpha", Path: "/a", LocationType: "project", Files: []string{}},
				{Name: "bravo", Path: "/b", LocationType: "project", Files: []string{}},
			},
			validateRows: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 3)
				// Column 1 is the name column (0 is the active-indicator).
				assert.Equal(t, "alpha", rows[0][1])
				assert.Equal(t, "bravo", rows[1][1])
				assert.Equal(t, "charlie", rows[2][1])
			},
		},
		{
			name: "different location types",
			profiles: []profile.ProfileInfo{
				{Name: "config", Path: "/c", LocationType: "configurable", Files: []string{}},
				{Name: "hidden", Path: "/h", LocationType: "project-hidden", Files: []string{}},
				{Name: "user", Path: "/u", LocationType: "xdg", Files: []string{}},
				{Name: "proj", Path: "/p", LocationType: "project", Files: []string{}},
			},
			validateRows: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 4)

				// Find each profile and verify location type.
				// Columns: [0]=indicator, [1]=name, [2]=location.
				locationTypes := make(map[string]string)
				for _, row := range rows {
					locationTypes[row[1]] = row[2]
				}

				assert.Equal(t, "configurable", locationTypes["config"])
				assert.Equal(t, "project-hidden", locationTypes["hidden"])
				assert.Equal(t, "xdg", locationTypes["user"])
				assert.Equal(t, "project", locationTypes["proj"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := buildProfileRows(tt.profiles, nil)

			require.NotNil(t, rows)

			if tt.validateRows != nil {
				// Convert table.Row to [][]string for easier testing.
				stringRows := make([][]string, len(rows))
				for i, row := range rows {
					stringRows[i] = row
				}
				tt.validateRows(t, stringRows)
			}
		})
	}
}

// TestBuildProfileRows_EdgeCases tests edge cases for buildProfileRows.
func TestBuildProfileRows_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		profiles []profile.ProfileInfo
		validate func(t *testing.T, rows [][]string)
	}{
		{
			name:     "empty profile list",
			profiles: []profile.ProfileInfo{},
			validate: func(t *testing.T, rows [][]string) {
				assert.Empty(t, rows)
			},
		},
		{
			name: "profile with special characters",
			profiles: []profile.ProfileInfo{
				{
					Name:         "profile-with-dashes_and_underscores",
					Path:         "/path/to/profile",
					LocationType: "project",
					Files:        []string{"file.yaml"},
				},
			},
			validate: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 1)
				// Column 1 is the name column (0 is the active-indicator).
				assert.Equal(t, "profile-with-dashes_and_underscores", rows[0][1])
			},
		},
		{
			name: "profile with unicode characters",
			profiles: []profile.ProfileInfo{
				{
					Name:         "profile-émoji-🚀",
					Path:         "/path/to/profile",
					LocationType: "project",
					Files:        []string{"file.yaml"},
				},
			},
			validate: func(t *testing.T, rows [][]string) {
				require.Len(t, rows, 1)
				assert.Contains(t, rows[0][1], "émoji")
			},
		},
		{
			name: "many profiles",
			profiles: func() []profile.ProfileInfo {
				var profiles []profile.ProfileInfo
				for i := 0; i < 50; i++ {
					profiles = append(profiles, profile.ProfileInfo{
						Name:         string(rune('a' + (i % 26))),
						Path:         "/path",
						LocationType: "project",
						Files:        []string{"file.yaml"},
					})
				}
				return profiles
			}(),
			validate: func(t *testing.T, rows [][]string) {
				assert.Equal(t, 50, len(rows))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := buildProfileRows(tt.profiles, nil)

			// Convert to string slices for validation.
			stringRows := make([][]string, len(rows))
			for i, row := range rows {
				stringRows[i] = row
			}

			tt.validate(t, stringRows)
		})
	}
}

// TestApplyTableStyles tests the applyTableStyles function.
func TestApplyTableStyles(t *testing.T) {
	profiles := []profile.ProfileInfo{
		{
			Name:         "test",
			Path:         "/path",
			LocationType: "project",
			Files:        []string{"atmos.yaml"},
		},
	}

	table, err := createProfilesTable(profiles, nil)
	require.NoError(t, err)

	// Verify table was created successfully and styles were applied.
	view := table.View()
	assert.NotEmpty(t, view)

	// Basic validation that table contains expected header and the single row.
	assert.Contains(t, view, "NAME")
	assert.Contains(t, view, "test")
}

// TestRenderTable_OutputFormat tests the overall output format.
func TestRenderTable_OutputFormat(t *testing.T) {
	profiles := []profile.ProfileInfo{
		{
			Name:         "production",
			Path:         "/path/to/production",
			LocationType: "project",
			Files:        []string{"atmos.yaml", "vpc.yaml"},
			Metadata: &schema.ConfigMetadata{
				Name:        "Production",
				Description: "Production environment",
			},
		},
		{
			Name:         "dev",
			Path:         "/path/to/dev",
			LocationType: "xdg",
			Files:        []string{"atmos.yaml"},
		},
	}

	// Mark "production" as the active profile so we can assert the indicator renders.
	output, err := RenderTable(profiles, map[string]bool{"production": true})
	require.NoError(t, err)

	// Verify structure.
	assert.Contains(t, output, "PROFILES", "Should have PROFILES header")

	// Verify column headers.
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "LOCATION")
	assert.Contains(t, output, "PATH")
	assert.Contains(t, output, "FILES")

	// Both profiles must be visible in the rendered table.
	assert.Contains(t, output, "production")
	assert.Contains(t, output, "dev")

	// The active profile gets a green dot indicator; inactive profiles do not.
	assert.Contains(t, output, "●", "active profile should have a dot indicator")
}
