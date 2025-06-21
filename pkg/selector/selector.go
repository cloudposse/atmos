package selector

import (
	"fmt"
	"regexp"
	"strings"
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
	reSet       = regexp.MustCompile(`^([A-Za-z0-9_\-]+)\s*(in|notin)\s*\(([^)]+)\)$`)
	reExists    = regexp.MustCompile(`^([A-Za-z0-9_\-]+)$`)
	reNotExists = regexp.MustCompile(`^!([A-Za-z0-9_\-]+)$`)
)

// Parse parses a label selector string and returns its requirements.
// Accepts comma-separated expressions. Whitespace is ignored around tokens.
func Parse(selector string) ([]Requirement, error) {
	if strings.TrimSpace(selector) == "" {
		return nil, fmt.Errorf("selector string is empty")
	}

	parts := splitSelector(selector)
	reqs := make([]Requirement, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if m := reEquality.FindStringSubmatch(p); len(m) == 4 {
			op := OpEqual
			if m[2] == "!=" {
				op = OpNotEqual
			}
			reqs = append(reqs, Requirement{Key: m[1], Operator: op, Values: []string{m[3]}})
			continue
		}
		if m := reSet.FindStringSubmatch(p); len(m) == 4 {
			op := OpIn
			if m[2] == "notin" {
				op = OpNotIn
			}
			vals := parseCSV(m[3])
			if len(vals) == 0 {
				return nil, fmt.Errorf("selector '%s': empty set values", p)
			}
			reqs = append(reqs, Requirement{Key: m[1], Operator: op, Values: vals})
			continue
		}
		if m := reExists.FindStringSubmatch(p); len(m) == 2 {
			reqs = append(reqs, Requirement{Key: m[1], Operator: OpExists})
			continue
		}
		if m := reNotExists.FindStringSubmatch(p); len(m) == 2 {
			reqs = append(reqs, Requirement{Key: m[1], Operator: OpNotExists})
			continue
		}
		return nil, fmt.Errorf("invalid selector expression: '%s'", p)
	}

	return reqs, nil
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
		if !exists {
			return false
		}
		for _, v := range r.Values {
			if val == v {
				return true
			}
		}
		return false
	case OpNotIn:
		if !exists {
			return true
		}
		for _, v := range r.Values {
			if val == v {
				return false
			}
		}
		return true
	case OpExists:
		return exists
	case OpNotExists:
		return !exists
	default:
		return false
	}
}
