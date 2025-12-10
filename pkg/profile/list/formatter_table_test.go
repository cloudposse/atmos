package list

import (
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
				// Note: Table rows don't appear in View() without focus.
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
				// Note: Profile data appears in table rows which are tested separately.
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
				// Note: File counts are tested in buildProfileRows tests.
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RenderTable(tt.profiles)

			require.NoError(t, err)
			require.NotEmpty(t, output)

			// Check for expected content.
			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected,
					"Output should contain: %s", expected)
			}

			// Note: Profile names in table rows may not appear in View() output
			// due to table library rendering behavior. We validate row building
			// separately in buildProfileRows tests.
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
				// Table View() only shows headers without focus.
				assert.Contains(t, output, "NAME")
				assert.Contains(t, output, "LOCATION")
				assert.Contains(t, output, "PATH")
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
				// Table rows are tested in buildProfileRows tests.
				// Here we just verify the table was created.
				assert.Contains(t, output, "NAME")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table, err := createProfilesTable(tt.profiles)

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
				assert.Equal(t, "dev", rows[0][0])
				assert.Equal(t, "project", rows[0][1])
				assert.Contains(t, rows[0][2], "/path/to/dev")
				assert.NotEmpty(t, rows[0][3]) // File count.
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
				path := rows[0][2]
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
				names := []string{rows[0][0], rows[1][0], rows[2][0], rows[3][0]}

				// Find each profile by name and verify file count.
				for i, name := range names {
					switch name {
					case "zero":
						assert.Equal(t, "0", rows[i][3], "Profile with no files should show 0")
					case "one":
						// The implementation uses: string(rune('0' + len(p.Files)))
						// For 1 file: '0' + 1 = '1'
						assert.Equal(t, "1", rows[i][3], "Profile with one file should show 1")
					case "nine":
						// For 9 files: '0' + 9 = '9'
						assert.Equal(t, "9", rows[i][3], "Profile with nine files should show 9")
					case "ten-plus":
						// For 10+ files, should show "10+".
						assert.Equal(t, "10+", rows[i][3], "Profile with 11 files should show 10+")
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
				assert.Equal(t, "alpha", rows[0][0])
				assert.Equal(t, "bravo", rows[1][0])
				assert.Equal(t, "charlie", rows[2][0])
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
				locationTypes := make(map[string]string)
				for _, row := range rows {
					locationTypes[row[0]] = row[1]
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
			rows := buildProfileRows(tt.profiles)

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
				assert.Equal(t, "profile-with-dashes_and_underscores", rows[0][0])
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
				assert.Contains(t, rows[0][0], "émoji")
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
			rows := buildProfileRows(tt.profiles)

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

	table, err := createProfilesTable(profiles)
	require.NoError(t, err)

	// Verify table was created successfully and styles were applied.
	view := table.View()
	assert.NotEmpty(t, view)

	// Basic validation that table contains expected header.
	assert.Contains(t, view, "NAME")

	// Note: Table rows may not appear in View() without focus/interaction.
	// The actual rendering is tested via RenderTable which includes full output.
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

	output, err := RenderTable(profiles)
	require.NoError(t, err)

	// Verify structure.
	assert.Contains(t, output, "PROFILES", "Should have PROFILES header")

	// Verify column headers.
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "LOCATION")
	assert.Contains(t, output, "PATH")
	assert.Contains(t, output, "FILES")

	// Note: The actual profile data may not appear in the table View() output
	// because the table library only renders headers without focus/interaction.
	// The important part is that the table structure is correct.
	// Testing profile data is covered in buildProfileRows tests.
}
