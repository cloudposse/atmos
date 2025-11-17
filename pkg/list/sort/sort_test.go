package sort

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNewSorter(t *testing.T) {
	sorter := NewSorter("Component", Ascending)
	assert.NotNil(t, sorter)
	assert.Equal(t, "Component", sorter.Column)
	assert.Equal(t, Ascending, sorter.Order)
	assert.Equal(t, String, sorter.DataType) // default
}

func TestSorter_WithDataType(t *testing.T) {
	sorter := NewSorter("Port", Ascending).WithDataType(Number)
	assert.Equal(t, Number, sorter.DataType)

	sorter = NewSorter("Enabled", Ascending).WithDataType(Boolean)
	assert.Equal(t, Boolean, sorter.DataType)
}

func TestSorter_Sort_Ascending(t *testing.T) {
	headers := []string{"Component", "Stack"}
	rows := [][]string{
		{"vpc", "prod"},
		{"eks", "dev"},
		{"rds", "staging"},
	}

	sorter := NewSorter("Component", Ascending)
	err := sorter.Sort(rows, headers)

	require.NoError(t, err)
	assert.Equal(t, "eks", rows[0][0])
	assert.Equal(t, "rds", rows[1][0])
	assert.Equal(t, "vpc", rows[2][0])
}

func TestSorter_Sort_Descending(t *testing.T) {
	headers := []string{"Component", "Stack"}
	rows := [][]string{
		{"vpc", "prod"},
		{"eks", "dev"},
		{"rds", "staging"},
	}

	sorter := NewSorter("Component", Descending)
	err := sorter.Sort(rows, headers)

	require.NoError(t, err)
	assert.Equal(t, "vpc", rows[0][0])
	assert.Equal(t, "rds", rows[1][0])
	assert.Equal(t, "eks", rows[2][0])
}

func TestSorter_Sort_Numeric(t *testing.T) {
	headers := []string{"Port", "Service"}
	rows := [][]string{
		{"8080", "app"},
		{"443", "https"},
		{"80", "http"},
		{"9090", "metrics"},
	}

	sorter := NewSorter("Port", Ascending).WithDataType(Number)
	err := sorter.Sort(rows, headers)

	require.NoError(t, err)
	assert.Equal(t, "80", rows[0][0])
	assert.Equal(t, "443", rows[1][0])
	assert.Equal(t, "8080", rows[2][0])
	assert.Equal(t, "9090", rows[3][0])
}

func TestSorter_Sort_Boolean(t *testing.T) {
	headers := []string{"Component", "Enabled"}
	rows := [][]string{
		{"vpc", "true"},
		{"eks", "false"},
		{"rds", "yes"},
		{"s3", "no"},
	}

	sorter := NewSorter("Enabled", Ascending).WithDataType(Boolean)
	err := sorter.Sort(rows, headers)

	require.NoError(t, err)
	// false values first (eks, s3), then true values (vpc, rds)
	assert.Contains(t, []string{"eks", "s3"}, rows[0][0])
	assert.Contains(t, []string{"eks", "s3"}, rows[1][0])
	assert.Contains(t, []string{"vpc", "rds"}, rows[2][0])
	assert.Contains(t, []string{"vpc", "rds"}, rows[3][0])
}

func TestSorter_Sort_ColumnNotFound(t *testing.T) {
	headers := []string{"Component"}
	rows := [][]string{{"vpc"}}

	sorter := NewSorter("NonExistent", Ascending)
	err := sorter.Sort(rows, headers)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestSorter_Sort_EmptyRows(t *testing.T) {
	headers := []string{"Component"}
	rows := [][]string{}

	sorter := NewSorter("Component", Ascending)
	err := sorter.Sort(rows, headers)

	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestCompareNumeric(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{"a < b", "5", "10", -1},
		{"a > b", "10", "5", 1},
		{"a == b", "5", "5", 0},
		{"decimal a < b", "1.5", "2.5", -1},
		{"negative numbers", "-5", "5", -1},
		{"non-numeric a", "abc", "5", 1},
		{"non-numeric b", "5", "abc", -1},
		{"both non-numeric", "abc", "xyz", -1}, // lexicographic fallback
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareNumeric(tt.a, tt.b)
			if tt.expected < 0 {
				assert.Less(t, result, 0)
			} else if tt.expected > 0 {
				assert.Greater(t, result, 0)
			} else {
				assert.Equal(t, 0, result)
			}
		})
	}
}

func TestCompareBoolean(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{"false < true", "false", "true", -1},
		{"true > false", "true", "false", 1},
		{"true == true", "true", "true", 0},
		{"false == false", "false", "false", 0},
		{"yes == true", "yes", "true", 0},
		{"no < yes", "no", "yes", -1},
		{"checkmark true", "✓", "✗", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareBoolean(tt.a, tt.b)
			if tt.expected < 0 {
				assert.Less(t, result, 0)
			} else if tt.expected > 0 {
				assert.Greater(t, result, 0)
			} else {
				assert.Equal(t, 0, result)
			}
		})
	}
}

func TestParseBoolean(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"yes", true},
		{"Yes", true},
		{"1", true},
		{"✓", true},
		{"false", false},
		{"False", false},
		{"no", false},
		{"0", false},
		{"✗", false},
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseBoolean(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewMultiSorter(t *testing.T) {
	sorter1 := NewSorter("Col1", Ascending)
	sorter2 := NewSorter("Col2", Descending)

	ms := NewMultiSorter(sorter1, sorter2)
	assert.NotNil(t, ms)
	assert.Len(t, ms.sorters, 2)
}

func TestMultiSorter_Sort(t *testing.T) {
	headers := []string{"Stack", "Component", "Region"}
	rows := [][]string{
		{"prod", "vpc", "us-east-1"},
		{"dev", "vpc", "us-west-2"},
		{"prod", "eks", "us-east-1"},
		{"dev", "eks", "us-west-2"},
	}

	// Sort by Stack (asc), then Component (asc)
	ms := NewMultiSorter(
		NewSorter("Stack", Ascending),
		NewSorter("Component", Ascending),
	)

	err := ms.Sort(rows, headers)
	require.NoError(t, err)

	// Expected order:
	// dev, eks, us-west-2
	// dev, vpc, us-west-2
	// prod, eks, us-east-1
	// prod, vpc, us-east-1
	assert.Equal(t, "dev", rows[0][0])
	assert.Equal(t, "eks", rows[0][1])
	assert.Equal(t, "dev", rows[1][0])
	assert.Equal(t, "vpc", rows[1][1])
	assert.Equal(t, "prod", rows[2][0])
	assert.Equal(t, "eks", rows[2][1])
	assert.Equal(t, "prod", rows[3][0])
	assert.Equal(t, "vpc", rows[3][1])
}

func TestMultiSorter_Sort_ColumnNotFound(t *testing.T) {
	headers := []string{"Component"}
	rows := [][]string{{"vpc"}}

	ms := NewMultiSorter(
		NewSorter("Component", Ascending),
		NewSorter("NonExistent", Ascending),
	)

	err := ms.Sort(rows, headers)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestParseSortSpec(t *testing.T) {
	tests := []struct {
		name          string
		spec          string
		expectedCount int
		expectErr     bool
		errType       error
	}{
		{"single column asc", "Component:asc", 1, false, nil},
		{"single column desc", "Component:desc", 1, false, nil},
		{"multiple columns", "Stack:asc,Component:desc", 2, false, nil},
		{"full words", "Stack:ascending,Component:descending", 2, false, nil},
		{"empty spec", "", 0, false, nil},
		{"whitespace handling", " Stack : asc , Component : desc ", 2, false, nil},
		{"missing colon", "Component", 0, true, errUtils.ErrInvalidConfig},
		{"invalid order", "Component:invalid", 0, true, errUtils.ErrInvalidConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorters, err := ParseSortSpec(tt.spec)

			if tt.expectErr {
				require.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				if tt.expectedCount == 0 {
					assert.Nil(t, sorters)
				} else {
					assert.Len(t, sorters, tt.expectedCount)
				}
			}
		})
	}
}

func TestParseSortSpec_OrderParsing(t *testing.T) {
	tests := []struct {
		spec          string
		expectedOrder Order
	}{
		{"Col:asc", Ascending},
		{"Col:ascending", Ascending},
		{"Col:desc", Descending},
		{"Col:descending", Descending},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			sorters, err := ParseSortSpec(tt.spec)
			require.NoError(t, err)
			require.Len(t, sorters, 1)
			assert.Equal(t, tt.expectedOrder, sorters[0].Order)
		})
	}
}

func TestSorter_Sort_StableSort(t *testing.T) {
	// Test that sorting is stable (preserves original order for equal elements)
	headers := []string{"Priority", "Name"}
	rows := [][]string{
		{"1", "Alice"},
		{"2", "Bob"},
		{"1", "Charlie"},
		{"2", "Diana"},
	}

	sorter := NewSorter("Priority", Ascending)
	err := sorter.Sort(rows, headers)

	require.NoError(t, err)
	// Items with same priority should maintain original order
	assert.Equal(t, "1", rows[0][0])
	assert.Equal(t, "Alice", rows[0][1])
	assert.Equal(t, "1", rows[1][0])
	assert.Equal(t, "Charlie", rows[1][1])
	assert.Equal(t, "2", rows[2][0])
	assert.Equal(t, "Bob", rows[2][1])
	assert.Equal(t, "2", rows[3][0])
	assert.Equal(t, "Diana", rows[3][1])
}
