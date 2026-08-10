// Package junit models JUnit XML test reports and provides encoding, decoding,
// and a markdown renderer. Aside from the repo-wide perf tracking helper it has
// no Atmos dependencies, so it can be reused by any producer of test results
// (terraform/opentofu test, the `junit` workflow step, etc.).
package junit

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Static errors (the repo bans dynamic errors; these keep the package dependency-free).
var (
	// ErrMarshal indicates the report could not be encoded to XML.
	ErrMarshal = errors.New("failed to encode JUnit XML")
	// ErrParse indicates the input could not be decoded as JUnit XML.
	ErrParse = errors.New("failed to parse JUnit XML")
)

// Report is the JUnit `<testsuites>` root: a collection of suites with rolled-up
// counts.
type Report struct {
	XMLName  xml.Name `xml:"testsuites"`
	Name     string   `xml:"name,attr,omitempty"`
	Tests    int      `xml:"tests,attr"`
	Failures int      `xml:"failures,attr"`
	Errors   int      `xml:"errors,attr"`
	Skipped  int      `xml:"skipped,attr"`
	Time     float64  `xml:"time,attr,omitempty"`
	Suites   []Suite  `xml:"testsuite"`
}

// Suite is a JUnit `<testsuite>` — typically one per test file.
type Suite struct {
	Name     string  `xml:"name,attr"`
	Tests    int     `xml:"tests,attr"`
	Failures int     `xml:"failures,attr"`
	Errors   int     `xml:"errors,attr"`
	Skipped  int     `xml:"skipped,attr"`
	Time     float64 `xml:"time,attr,omitempty"`
	Cases    []Case  `xml:"testcase"`
}

// Case is a JUnit `<testcase>` — one per test/run. Exactly one of Failure,
// Error, or Skipped is set for non-passing cases; all nil means the case passed.
// File/Line are optional attributes many CI test reporters and editors honor.
type Case struct {
	Name      string  `xml:"name,attr"`
	Classname string  `xml:"classname,attr,omitempty"`
	File      string  `xml:"file,attr,omitempty"`
	Line      int     `xml:"line,attr,omitempty"`
	Time      float64 `xml:"time,attr,omitempty"`
	Failure   *Detail `xml:"failure,omitempty"`
	Error     *Detail `xml:"error,omitempty"`
	Skipped   *Detail `xml:"skipped,omitempty"`
}

// Detail is the body of a `<failure>`, `<error>`, or `<skipped>` element:
// a short Message attribute plus optional Text content.
type Detail struct {
	Message string `xml:"message,attr,omitempty"`
	Type    string `xml:"type,attr,omitempty"`
	Text    string `xml:",chardata"`
}

// Status reports whether a case passed, failed, errored, or was skipped.
func (c *Case) Status() string {
	defer perf.Track(nil, "junit.Case.Status")()

	switch {
	case c.Error != nil:
		return "error"
	case c.Failure != nil:
		return "fail"
	case c.Skipped != nil:
		return "skip"
	default:
		return "pass"
	}
}

// Aggregate recomputes each suite's counts from its cases and the report's
// totals from its suites, so callers can build a Report by appending cases and
// let the library fill in the attribute counts.
func (r *Report) Aggregate() {
	defer perf.Track(nil, "junit.Report.Aggregate")()

	r.Tests, r.Failures, r.Errors, r.Skipped, r.Time = 0, 0, 0, 0, 0
	for i := range r.Suites {
		s := &r.Suites[i]
		s.Tests, s.Failures, s.Errors, s.Skipped = 0, 0, 0, 0
		for _, c := range s.Cases {
			s.Tests++
			switch c.Status() {
			case "fail":
				s.Failures++
			case "error":
				s.Errors++
			case "skip":
				s.Skipped++
			}
		}
		r.Tests += s.Tests
		r.Failures += s.Failures
		r.Errors += s.Errors
		r.Skipped += s.Skipped
		r.Time += s.Time
	}
}

// Passed reports whether every case passed (no failures or errors). Skips do not
// count as failures.
func (r *Report) Passed() bool {
	defer perf.Track(nil, "junit.Report.Passed")()

	return r.Failures == 0 && r.Errors == 0
}

// Format encodes the report as indented JUnit XML with an XML header.
func Format(r *Report) ([]byte, error) {
	defer perf.Track(nil, "junit.Format")()

	body, err := xml.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMarshal, err)
	}
	// Build header + body + trailing newline via append so the allocation size is
	// driven by a single length (xml.Header is a constant), avoiding a len()+len()
	// sum that CodeQL flags as a potential allocation-size overflow.
	out := append([]byte(xml.Header), body...)
	out = append(out, '\n')
	return out, nil
}

// Parse decodes JUnit XML into a Report. It accepts both a `<testsuites>` root
// and a bare `<testsuite>` root (which is wrapped into a single-suite report),
// since test runners differ on which they emit.
func Parse(data []byte) (Report, error) {
	defer perf.Track(nil, "junit.Parse")()

	var r Report
	if err := xml.Unmarshal(data, &r); err == nil && (len(r.Suites) > 0 || rootIs(data, "testsuites")) {
		return r, nil
	}

	// Fall back to a bare <testsuite> root.
	var s Suite
	if err := xml.Unmarshal(data, &s); err != nil {
		return Report{}, fmt.Errorf("%w: %w", ErrParse, err)
	}
	return Report{Suites: []Suite{s}}, nil
}

// rootIs reports whether the document's first element is the given local name.
func rootIs(data []byte, name string) bool {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local == name
		}
	}
}

// FailureRef is a flattened reference to a failing/errored case, for callers
// that build CI annotations (which live outside this dependency-free package).
type FailureRef struct {
	Suite   string
	Name    string
	File    string
	Line    int
	Message string
}

// FailedCases returns one FailureRef per failing or errored case across all
// suites (named to avoid colliding with the Failures count field).
func (r *Report) FailedCases() []FailureRef {
	defer perf.Track(nil, "junit.Report.FailedCases")()

	var out []FailureRef
	for _, s := range r.Suites {
		for _, c := range s.Cases {
			detail := c.Failure
			if detail == nil {
				detail = c.Error
			}
			if detail == nil {
				continue
			}
			out = append(out, FailureRef{
				Suite:   s.Name,
				Name:    c.Name,
				File:    c.File,
				Line:    c.Line,
				Message: firstNonEmpty(detail.Message, strings.TrimSpace(detail.Text)),
			})
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
