package selector

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// Static errors for wrapping.
	ErrEmptySelector     = errors.New("selector string is empty")
	ErrEmptySetValues    = errors.New("empty set values")
	ErrInvalidExpression = errors.New("invalid selector expression")
)

// Operator represents a selector operator.
// Supported values: =, !=, in, notin, exists, notexists.
// For exists and notexists, the Values slice is empty.
// Key is always populated.
// Inspiration: Kubernetes label selector semantics.
type Operator string

const (
	OpEqual     Operator = "="
	OpNotEqual  Operator = "!="
	OpIn        Operator = "in"
	OpNotIn     Operator = "notin"
	OpExists    Operator = "exists"
	OpNotExists Operator = "notexists"
)

// Requirement represents a single selector requirement: key op values.
type Requirement struct {
	Key      string
	Operator Operator
	Values   []string
}

var (
	// Regex patterns to capture selector tokens.
	reEquality  = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*(=|!=)\s*([A-Za-z0-9_\-]+)$`)
	reSet       = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*(in|notin)\s*\(([^)]*)\)$`)
	reExists    = regexp.MustCompile(`^([A-Za-z0-9_\-]+)$`)
	reNotExists = regexp.MustCompile(`^!([A-Za-z0-9_\-]+)$`)
)

// Parse parses a label selector string and returns its requirements.
// Accepts comma-separated expressions. Whitespace is ignored around tokens.
func Parse(selector string) ([]Requirement, error) {
	if strings.TrimSpace(selector) == "" {
		return nil, ErrEmptySelector
	}

	parts := splitSelector(selector)
	reqs := make([]Requirement, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		req, err := parseExpression(p)
		if err != nil {
			return nil, err
		}

		reqs = append(reqs, req)
	}

	return reqs, nil
}

// parseExpression parses a single selector expression and returns a Requirement.
func parseExpression(expr string) (Requirement, error) {
	// Try equality/inequality patterns
	if req, ok := parseEqualityExpression(expr); ok {
		return req, nil
	}

	// Try set patterns (in/notin)
	if req, err := parseSetExpression(expr); err == nil {
		return req, nil
	} else if !errors.Is(err, ErrInvalidExpression) {
		return Requirement{}, err
	}

	// Try exists pattern
	if req, ok := parseExistsExpression(expr); ok {
		return req, nil
	}

	// Try not-exists pattern
	if req, ok := parseNotExistsExpression(expr); ok {
		return req, nil
	}

	return Requirement{}, fmt.Errorf("'%s': %w", expr, ErrInvalidExpression)
}

// parseEqualityExpression parses equality/inequality expressions (key=value, key!=value).
func parseEqualityExpression(expr string) (Requirement, bool) {
	m := reEquality.FindStringSubmatch(expr)
	if len(m) != 4 {
		return Requirement{}, false
	}

	op := OpEqual
	if m[2] == "!=" {
		op = OpNotEqual
	}

	return Requirement{Key: m[1], Operator: op, Values: []string{m[3]}}, true
}

// parseSetExpression parses set expressions (key in (a,b), key notin (a,b)).
func parseSetExpression(expr string) (Requirement, error) {
	m := reSet.FindStringSubmatch(expr)
	if len(m) != 4 {
		return Requirement{}, ErrInvalidExpression
	}

	op := OpIn
	if m[2] == "notin" {
		op = OpNotIn
	}

	vals := parseCSV(m[3])
	if len(vals) == 0 {
		return Requirement{}, fmt.Errorf("selector '%s': %w", expr, ErrEmptySetValues)
	}

	return Requirement{Key: m[1], Operator: op, Values: vals}, nil
}

// parseExistsExpression parses exists expressions (key).
func parseExistsExpression(expr string) (Requirement, bool) {
	m := reExists.FindStringSubmatch(expr)
	if len(m) != 2 {
		return Requirement{}, false
	}

	return Requirement{Key: m[1], Operator: OpExists}, true
}

// parseNotExistsExpression parses not-exists expressions (!key).
func parseNotExistsExpression(expr string) (Requirement, bool) {
	m := reNotExists.FindStringSubmatch(expr)
	if len(m) != 2 {
		return Requirement{}, false
	}

	return Requirement{Key: m[1], Operator: OpNotExists}, true
}

func splitSelector(sel string) []string {
	var parts []string
	depth := 0
	last := 0
	for i, r := range sel {
		switch r {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, sel[last:i])
				last = i + 1
			}
		}
	}
	parts = append(parts, sel[last:])
	return parts
}

func parseCSV(s string) []string {
	items := strings.Split(s, ",")
	var out []string
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it != "" {
			out = append(out, it)
		}
	}
	return out
}

// Matches returns true if the labels map satisfies all requirements.
func Matches(labels map[string]string, reqs []Requirement) bool {
	for _, r := range reqs {
		if !matchRequirement(labels, r) {
			return false
		}
	}
	return true
}

func matchRequirement(labels map[string]string, r Requirement) bool {
	val, exists := labels[r.Key]
	switch r.Operator {
	case OpEqual:
		return exists && val == r.Values[0]
	case OpNotEqual:
		return !exists || val != r.Values[0]
	case OpIn:
		return matchInRequirement(val, exists, r.Values)
	case OpNotIn:
		return matchNotInRequirement(val, exists, r.Values)
	case OpExists:
		return exists
	case OpNotExists:
		return !exists
	default:
		return false
	}
}

// matchInRequirement checks if the value exists and is in the provided set of values.
func matchInRequirement(val string, exists bool, values []string) bool {
	if !exists {
		return false
	}
	for _, v := range values {
		if val == v {
			return true
		}
	}
	return false
}

// matchNotInRequirement checks if the value doesn't exist or is not in the provided set of values.
func matchNotInRequirement(val string, exists bool, values []string) bool {
	if !exists {
		return true
	}
	for _, v := range values {
		if val == v {
			return false
		}
	}
	return true
}
