package sort

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Order defines sort direction.
type Order int

const (
	// Ascending sort order.
	Ascending Order = iota
	// Descending sort order.
	Descending
)

// DataType defines the type of data for type-aware sorting.
type DataType int

const (
	// String data type (lexicographic sorting).
	String DataType = iota
	// Number data type (numeric sorting).
	Number
	// Boolean data type (false < true).
	Boolean
)

// Sorter handles single column sorting.
type Sorter struct {
	Column   string
	Order    Order
	DataType DataType
}

// MultiSorter handles multi-column sorting with precedence.
type MultiSorter struct {
	sorters []*Sorter
}

// NewSorter creates a sorter for a single column.
// DataType is set to String by default, use WithDataType() to override.
func NewSorter(column string, order Order) *Sorter {
	return &Sorter{
		Column:   column,
		Order:    order,
		DataType: String,
	}
}

// WithDataType sets explicit data type for type-aware sorting.
func (s *Sorter) WithDataType(dt DataType) *Sorter {
	s.DataType = dt
	return s
}

// Sort sorts rows in-place by the column.
func (s *Sorter) Sort(rows [][]string, headers []string) error {
	// Find column index
	colIdx := -1
	for i, h := range headers {
		if h == s.Column {
			colIdx = i
			break
		}
	}

	if colIdx == -1 {
		return fmt.Errorf("%w: column %q not found in headers", errUtils.ErrInvalidConfig, s.Column)
	}

	// Sort with type-aware comparison
	sort.SliceStable(rows, func(i, j int) bool {
		if colIdx >= len(rows[i]) || colIdx >= len(rows[j]) {
			return false
		}

		valI := rows[i][colIdx]
		valJ := rows[j][colIdx]

		cmp := s.compare(valI, valJ)

		if s.Order == Ascending {
			return cmp < 0
		}
		return cmp > 0
	})

	return nil
}

// compare performs type-aware comparison.
// Returns: -1 if a < b, 0 if a == b, 1 if a > b.
func (s *Sorter) compare(a, b string) int {
	switch s.DataType {
	case Number:
		return compareNumeric(a, b)
	case Boolean:
		return compareBoolean(a, b)
	default: // String
		return strings.Compare(a, b)
	}
}

// compareNumeric compares two strings as numbers.
func compareNumeric(a, b string) int {
	numA, errA := strconv.ParseFloat(a, 64)
	numB, errB := strconv.ParseFloat(b, 64)

	// Non-numeric values sort last
	if errA != nil && errB != nil {
		return strings.Compare(a, b)
	}
	if errA != nil {
		return 1
	}
	if errB != nil {
		return -1
	}

	if numA < numB {
		return -1
	}
	if numA > numB {
		return 1
	}
	return 0
}

// compareBoolean compares two strings as booleans (false < true).
func compareBoolean(a, b string) int {
	boolA := parseBoolean(a)
	boolB := parseBoolean(b)

	if !boolA && boolB {
		return -1
	}
	if boolA && !boolB {
		return 1
	}
	return 0
}

// parseBoolean converts string to boolean.
func parseBoolean(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return lower == "true" || lower == "yes" || lower == "1" || lower == "✓"
}

// NewMultiSorter creates a multi-column sorter.
// Sorters are applied in order (primary, secondary, etc.).
func NewMultiSorter(sorters ...*Sorter) *MultiSorter {
	return &MultiSorter{sorters: sorters}
}

// Sort applies all sorters in order with stable sorting.
func (ms *MultiSorter) Sort(rows [][]string, headers []string) error {
	// Validate all sorters
	for i, s := range ms.sorters {
		colIdx := -1
		for j, h := range headers {
			if h == s.Column {
				colIdx = j
				break
			}
		}
		if colIdx == -1 {
			return fmt.Errorf("%w: sorter %d: column %q not found", errUtils.ErrInvalidConfig, i, s.Column)
		}
	}

	// Apply sorters in reverse order for stable multi-column sorting
	// This ensures primary sort takes precedence
	for i := len(ms.sorters) - 1; i >= 0; i-- {
		if err := ms.sorters[i].Sort(rows, headers); err != nil {
			return err
		}
	}

	return nil
}

// ParseSortSpec parses CLI sort specification.
// Format: "column1:asc,column2:desc" or "column1:ascending,column2:descending".
func ParseSortSpec(spec string) ([]*Sorter, error) {
	if spec == "" {
		return nil, nil
	}

	parts := strings.Split(spec, ",")
	var sorters []*Sorter

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Split by colon
		fields := strings.SplitN(part, ":", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%w: invalid sort spec %q, expected format 'column:order'", errUtils.ErrInvalidConfig, part)
		}

		column := strings.TrimSpace(fields[0])
		// Normalize column name: capitalize first letter for case-insensitive matching.
		// This allows users to use "stack:asc" instead of requiring "Stack:asc".
		if len(column) > 0 {
			column = strings.ToUpper(column[:1]) + column[1:]
		}
		orderStr := strings.ToLower(strings.TrimSpace(fields[1]))

		var order Order
		switch orderStr {
		case "asc", "ascending":
			order = Ascending
		case "desc", "descending":
			order = Descending
		default:
			return nil, fmt.Errorf("%w: invalid sort order %q, expected 'asc' or 'desc'", errUtils.ErrInvalidConfig, orderStr)
		}

		sorters = append(sorters, NewSorter(column, order))
	}

	return sorters, nil
}
