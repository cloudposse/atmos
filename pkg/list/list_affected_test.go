package list

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseAffectedColumnsFlag(t *testing.T) {
	tests := []struct {
		name           string
		columnsFlag    []string
		expectedLen    int
		expectedFirst  *column.Config
		expectDefaults bool
	}{
		{
			name:           "empty columns flag returns defaults",
			columnsFlag:    []string{},
			expectedLen:    len(tableAffectedColumns),
			expectDefaults: true,
		},
		{
			name:           "nil columns flag returns defaults",
			columnsFlag:    nil,
			expectedLen:    len(tableAffectedColumns),
			expectDefaults: true,
		},
		{
			name:        "single valid spec",
			columnsFlag: []string{"Component={{ .component }}"},
			expectedLen: 1,
			expectedFirst: &column.Config{
				Name:  "Component",
				Value: "{{ .component }}",
			},
		},
		{
			name:        "multiple valid specs",
			columnsFlag: []string{"Component={{ .component }}", "Stack={{ .stack }}"},
			expectedLen: 2,
			expectedFirst: &column.Config{
				Name:  "Component",
				Value: "{{ .component }}",
			},
		},
		{
			name:           "invalid spec without equals sign is skipped",
			columnsFlag:    []string{"InvalidSpec"},
			expectedLen:    len(tableAffectedColumns),
			expectDefaults: true,
		},
		{
			name:           "empty name is skipped",
			columnsFlag:    []string{"={{ .value }}"},
			expectedLen:    len(tableAffectedColumns),
			expectDefaults: true,
		},
		{
			name:           "empty value is skipped",
			columnsFlag:    []string{"Name="},
			expectedLen:    len(tableAffectedColumns),
			expectDefaults: true,
		},
		{
			name:        "mixed valid and invalid specs",
			columnsFlag: []string{"Valid={{ .val }}", "Invalid", "Also Valid={{ .also }}"},
			expectedLen: 2,
			expectedFirst: &column.Config{
				Name:  "Valid",
				Value: "{{ .val }}",
			},
		},
		{
			name:        "spec with multiple equals signs uses first",
			columnsFlag: []string{"Name=value=with=equals"},
			expectedLen: 1,
			expectedFirst: &column.Config{
				Name:  "Name",
				Value: "value=with=equals",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAffectedColumnsFlag(tt.columnsFlag)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectedLen)

			if tt.expectDefaults {
				assert.Equal(t, tableAffectedColumns, result)
			}

			if tt.expectedFirst != nil && len(result) > 0 {
				assert.Equal(t, tt.expectedFirst.Name, result[0].Name)
				assert.Equal(t, tt.expectedFirst.Value, result[0].Value)
			}
		})
	}
}

func TestSelectRemoteRef(t *testing.T) {
	tests := []struct {
		name     string
		sha      string
		ref      string
		expected string
	}{
		{
			name:     "SHA takes precedence over ref",
			sha:      "abc123",
			ref:      "refs/heads/main",
			expected: "abc123",
		},
		{
			name:     "ref used when SHA is empty",
			sha:      "",
			ref:      "refs/heads/main",
			expected: "refs/heads/main",
		},
		{
			name:     "default fallback when both empty",
			sha:      "",
			ref:      "",
			expected: "refs/remotes/origin/HEAD",
		},
		{
			name:     "SHA only",
			sha:      "deadbeef",
			ref:      "",
			expected: "deadbeef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectRemoteRef(tt.sha, tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetRefNames(t *testing.T) {
	tests := []struct {
		name           string
		repoPath       string
		sha            string
		ref            string
		localHead      *plumbing.Reference
		expectedLocal  string
		expectedRemote string
	}{
		{
			name:           "RepoPath sets local to HEAD and remote to path",
			repoPath:       "/path/to/repo",
			expectedLocal:  "HEAD",
			expectedRemote: "/path/to/repo",
		},
		{
			name:           "localHead not nil sets LocalRef from head",
			repoPath:       "",
			sha:            "",
			ref:            "refs/heads/main",
			localHead:      plumbing.NewReferenceFromStrings("refs/heads/feature", "abc123"),
			expectedLocal:  "refs/heads/feature",
			expectedRemote: "refs/heads/main",
		},
		{
			name:           "localHead nil leaves LocalRef empty",
			repoPath:       "",
			sha:            "deadbeef",
			ref:            "",
			localHead:      nil,
			expectedLocal:  "",
			expectedRemote: "deadbeef",
		},
		{
			name:           "fallback to default remote ref",
			repoPath:       "",
			sha:            "",
			ref:            "",
			localHead:      nil,
			expectedLocal:  "",
			expectedRemote: "refs/remotes/origin/HEAD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &affectedResult{}
			opts := &AffectedCommandOptions{
				RepoPath: tt.repoPath,
				SHA:      tt.sha,
				Ref:      tt.ref,
			}

			setRefNames(result, opts, tt.localHead)

			assert.Equal(t, tt.expectedLocal, result.LocalRef)
			assert.Equal(t, tt.expectedRemote, result.RemoteRef)
		})
	}
}

func TestBuildAffectedFilters(t *testing.T) {
	tests := []struct {
		name       string
		filterSpec string
		expectNil  bool
		expectLen  int
	}{
		{
			name:       "empty spec returns nil",
			filterSpec: "",
			expectNil:  true,
		},
		{
			name:       "valid spec with colon delimiter",
			filterSpec: "component:vpc",
			expectNil:  false,
			expectLen:  1,
		},
		{
			name:       "valid spec with equals delimiter",
			filterSpec: "stack=dev",
			expectNil:  false,
			expectLen:  1,
		},
		{
			name:       "invalid spec without delimiter returns nil",
			filterSpec: "invalidspec",
			expectNil:  true,
		},
		{
			name:       "delimiter at start returns nil",
			filterSpec: ":value",
			expectNil:  true,
		},
		{
			name:       "delimiter at end returns nil",
			filterSpec: "field:",
			expectNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildAffectedFilters(tt.filterSpec)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.Len(t, result, tt.expectLen)
			}
		})
	}
}

func TestBuildAffectedSorters(t *testing.T) {
	columnsWithStackAndComponent := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Component", Value: "{{ .component }}"},
	}

	columnsWithoutRequired := []column.Config{
		{Name: "Type", Value: "{{ .component_type }}"},
	}

	tests := []struct {
		name            string
		sortSpec        string
		columns         []column.Config
		expectNil       bool
		expectLen       int
		firstCol        string
		checkFirstOrder bool
		firstOrder      listSort.Order
	}{
		{
			name:            "empty spec with Stack and Component returns defaults",
			sortSpec:        "",
			columns:         columnsWithStackAndComponent,
			expectNil:       false,
			expectLen:       2,
			firstCol:        "Stack",
			checkFirstOrder: true,
			firstOrder:      listSort.Ascending,
		},
		{
			name:      "empty spec without required columns returns nil",
			sortSpec:  "",
			columns:   columnsWithoutRequired,
			expectNil: true,
		},
		{
			name:      "explicit sort spec delegates to ParseSortSpec",
			sortSpec:  "Component:asc",
			columns:   columnsWithStackAndComponent,
			expectNil: false,
			expectLen: 1,
			firstCol:  "Component",
		},
		{
			name:      "only Stack column returns nil",
			sortSpec:  "",
			columns:   []column.Config{{Name: "Stack", Value: "{{ .stack }}"}},
			expectNil: true,
		},
		{
			name:      "only Component column returns nil",
			sortSpec:  "",
			columns:   []column.Config{{Name: "Component", Value: "{{ .component }}"}},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildAffectedSorters(tt.sortSpec, tt.columns)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.Len(t, result, tt.expectLen)
				assert.Equal(t, tt.firstCol, result[0].Column)
				if tt.checkFirstOrder {
					assert.Equal(t, tt.firstOrder, result[0].Order)
				}
			}
		})
	}
}

func TestGetAffectedColumns(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name        string
		columnsFlag []string
		formatFlag  string
		expected    []column.Config
	}{
		{
			name:        "columnsFlag provided returns parsed columns",
			columnsFlag: []string{"Custom={{ .custom }}"},
			formatFlag:  string(format.FormatTable),
			expected:    []column.Config{{Name: "Custom", Value: "{{ .custom }}"}},
		},
		{
			name:        "JSON format returns data columns",
			columnsFlag: nil,
			formatFlag:  string(format.FormatJSON),
			expected:    dataAffectedColumns,
		},
		{
			name:        "YAML format returns data columns",
			columnsFlag: nil,
			formatFlag:  string(format.FormatYAML),
			expected:    dataAffectedColumns,
		},
		{
			name:        "CSV format returns data columns",
			columnsFlag: nil,
			formatFlag:  string(format.FormatCSV),
			expected:    dataAffectedColumns,
		},
		{
			name:        "TSV format returns data columns",
			columnsFlag: nil,
			formatFlag:  string(format.FormatTSV),
			expected:    dataAffectedColumns,
		},
		{
			name:        "table format returns table columns",
			columnsFlag: nil,
			formatFlag:  string(format.FormatTable),
			expected:    tableAffectedColumns,
		},
		{
			name:        "empty format returns table columns",
			columnsFlag: nil,
			formatFlag:  "",
			expected:    tableAffectedColumns,
		},
		{
			name:        "invalid columnsFlag falls back to table defaults from parseAffectedColumnsFlag",
			columnsFlag: []string{"InvalidNoEquals"},
			formatFlag:  string(format.FormatJSON),
			expected:    tableAffectedColumns, // parseAffectedColumnsFlag returns defaults when parsing fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAffectedColumns(atmosConfig, tt.columnsFlag, tt.formatFlag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAffectedResult(t *testing.T) {
	t.Run("affectedResult struct fields", func(t *testing.T) {
		result := &affectedResult{
			Affected:     []schema.Affected{{Component: "vpc", Stack: "dev"}},
			LocalRef:     "refs/heads/feature",
			RemoteRef:    "refs/heads/main",
			RemoteRepoID: "repo-123",
		}

		assert.Len(t, result.Affected, 1)
		assert.Equal(t, "vpc", result.Affected[0].Component)
		assert.Equal(t, "refs/heads/feature", result.LocalRef)
		assert.Equal(t, "refs/heads/main", result.RemoteRef)
		assert.Equal(t, "repo-123", result.RemoteRepoID)
	})
}

func TestAffectedLogicResult(t *testing.T) {
	t.Run("affectedLogicResult struct fields", func(t *testing.T) {
		head := plumbing.NewReferenceFromStrings("refs/heads/main", "abc123")
		result := &affectedLogicResult{
			affected:     []schema.Affected{{Component: "eks", Stack: "prod"}},
			localHead:    head,
			remoteRepoID: "repo-456",
		}

		assert.Len(t, result.affected, 1)
		assert.Equal(t, "eks", result.affected[0].Component)
		assert.NotNil(t, result.localHead)
		assert.Equal(t, "repo-456", result.remoteRepoID)
	})
}

func TestTableAndDataAffectedColumns(t *testing.T) {
	t.Run("tableAffectedColumns has expected structure", func(t *testing.T) {
		assert.Len(t, tableAffectedColumns, 6)
		assert.Equal(t, " ", tableAffectedColumns[0].Name)
		assert.Equal(t, "Component", tableAffectedColumns[1].Name)
		assert.Equal(t, "Stack", tableAffectedColumns[2].Name)
		assert.Equal(t, "Type", tableAffectedColumns[3].Name)
		assert.Equal(t, "Affected", tableAffectedColumns[4].Name)
		assert.Equal(t, "File", tableAffectedColumns[5].Name)
	})

	t.Run("dataAffectedColumns has expected structure", func(t *testing.T) {
		assert.Len(t, dataAffectedColumns, 6)
		assert.Equal(t, "Status", dataAffectedColumns[0].Name)
		assert.Equal(t, "Component", dataAffectedColumns[1].Name)
		assert.Equal(t, "Stack", dataAffectedColumns[2].Name)
		assert.Equal(t, "Type", dataAffectedColumns[3].Name)
		assert.Equal(t, "Affected", dataAffectedColumns[4].Name)
		assert.Equal(t, "File", dataAffectedColumns[5].Name)
	})

	t.Run("data columns use status_text instead of status", func(t *testing.T) {
		assert.Contains(t, dataAffectedColumns[0].Value, "status_text")
		assert.Contains(t, tableAffectedColumns[0].Value, "status")
		assert.NotContains(t, tableAffectedColumns[0].Value, "status_text")
	})
}

func TestBuildAffectedFilters_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		filterSpec string
		expectNil  bool
		expectLen  int
	}{
		{
			name:       "filter with multiple colons uses first",
			filterSpec: "component:vpc:extra",
			expectNil:  false,
			expectLen:  1,
		},
		{
			name:       "filter with spaces",
			filterSpec: "stack:dev stack",
			expectNil:  false,
			expectLen:  1,
		},
		{
			name:       "filter with special characters",
			filterSpec: "component:vpc-*",
			expectNil:  false,
			expectLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildAffectedFilters(tt.filterSpec)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.Len(t, result, tt.expectLen)
			}
		})
	}
}

func TestBuildAffectedSorters_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		sortSpec  string
		columns   []column.Config
		expectNil bool
		expectLen int
	}{
		{
			name:      "descending sort",
			sortSpec:  "Stack:desc",
			columns:   []column.Config{{Name: "Stack", Value: "{{ .stack }}"}},
			expectNil: false,
			expectLen: 1,
		},
		{
			name:      "multiple sort columns",
			sortSpec:  "Stack:asc,Component:desc",
			columns:   []column.Config{{Name: "Stack", Value: "{{ .stack }}"}, {Name: "Component", Value: "{{ .component }}"}},
			expectNil: false,
			expectLen: 2,
		},
		{
			name:      "empty columns list",
			sortSpec:  "",
			columns:   []column.Config{},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildAffectedSorters(tt.sortSpec, tt.columns)
			require.NoError(t, err)

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.Len(t, result, tt.expectLen)
			}
		})
	}
}

func TestParseAffectedColumnsFlag_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		columnsFlag []string
		expectedLen int
	}{
		{
			name:        "column with complex template",
			columnsFlag: []string{"Status={{ if .enabled }}enabled{{ else }}disabled{{ end }}"},
			expectedLen: 1,
		},
		{
			name:        "column with dots in value",
			columnsFlag: []string{"Info={{ .component }}.{{ .stack }}"},
			expectedLen: 1,
		},
		{
			name:        "column with unicode",
			columnsFlag: []string{"状态={{ .status }}"},
			expectedLen: 1,
		},
		{
			name:        "all invalid columns return defaults",
			columnsFlag: []string{"invalid1", "invalid2", "invalid3"},
			expectedLen: len(tableAffectedColumns),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseAffectedColumnsFlag(tt.columnsFlag)
			require.NoError(t, err)
			assert.Len(t, result, tt.expectedLen)
		})
	}
}

func TestSelectRemoteRef_Precedence(t *testing.T) {
	// Test that SHA always takes precedence, then ref, then default.
	t.Run("SHA precedence is absolute", func(t *testing.T) {
		result := selectRemoteRef("sha123", "refs/heads/main")
		assert.Equal(t, "sha123", result)
	})

	t.Run("ref used when no SHA", func(t *testing.T) {
		result := selectRemoteRef("", "refs/heads/feature")
		assert.Equal(t, "refs/heads/feature", result)
	})

	t.Run("default when nothing provided", func(t *testing.T) {
		result := selectRemoteRef("", "")
		assert.Equal(t, "refs/remotes/origin/HEAD", result)
	})
}

func TestSetRefNames_Combinations(t *testing.T) {
	t.Run("RepoPath always overrides other settings", func(t *testing.T) {
		result := &affectedResult{}
		head := plumbing.NewReferenceFromStrings("refs/heads/feature", "abc123")
		opts := &AffectedCommandOptions{
			RepoPath: "/some/path",
			SHA:      "sha123",
			Ref:      "refs/heads/main",
		}

		setRefNames(result, opts, head)

		assert.Equal(t, "HEAD", result.LocalRef)
		assert.Equal(t, "/some/path", result.RemoteRef)
	})

	t.Run("SHA takes precedence over ref in remote", func(t *testing.T) {
		result := &affectedResult{}
		opts := &AffectedCommandOptions{
			SHA: "sha456",
			Ref: "refs/heads/main",
		}

		setRefNames(result, opts, nil)

		assert.Equal(t, "", result.LocalRef)
		assert.Equal(t, "sha456", result.RemoteRef)
	})
}

func TestAffectedCommandOptions(t *testing.T) {
	t.Run("AffectedCommandOptions struct initialization", func(t *testing.T) {
		opts := &AffectedCommandOptions{
			ColumnsFlag:       []string{"Col={{ .col }}"},
			FilterSpec:        "component:vpc",
			SortSpec:          "Stack:asc",
			Delimiter:         ",",
			Ref:               "refs/heads/main",
			SHA:               "abc123",
			RepoPath:          "/path/to/repo",
			SSHKeyPath:        "/path/to/key",
			SSHKeyPassword:    "secret",
			CloneTargetRef:    true,
			IncludeDependents: true,
			Stack:             "dev",
			ProcessTemplates:  true,
			ProcessFunctions:  true,
			Skip:              []string{"skip1", "skip2"},
			ExcludeLocked:     true,
		}

		assert.Equal(t, []string{"Col={{ .col }}"}, opts.ColumnsFlag)
		assert.Equal(t, "component:vpc", opts.FilterSpec)
		assert.Equal(t, "Stack:asc", opts.SortSpec)
		assert.Equal(t, ",", opts.Delimiter)
		assert.Equal(t, "refs/heads/main", opts.Ref)
		assert.Equal(t, "abc123", opts.SHA)
		assert.Equal(t, "/path/to/repo", opts.RepoPath)
		assert.Equal(t, "/path/to/key", opts.SSHKeyPath)
		assert.Equal(t, "secret", opts.SSHKeyPassword)
		assert.True(t, opts.CloneTargetRef)
		assert.True(t, opts.IncludeDependents)
		assert.Equal(t, "dev", opts.Stack)
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
		assert.Equal(t, []string{"skip1", "skip2"}, opts.Skip)
		assert.True(t, opts.ExcludeLocked)
	})
}
