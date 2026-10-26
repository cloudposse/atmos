// Package sarif parses SARIF 2.1.0 documents into a normalized Findings
// representation usable by hook ResultHandlers. It is intentionally lenient:
// tools emit subtly different SARIF (rule ID format, level mapping, location
// shape), so the parser tolerates missing or unusual fields rather than
// erroring out.
//
// Reference spec: https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html
package sarif

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Severity is the normalized severity of a finding. It maps SARIF's
// `level` + `properties.severity` into a single Atmos-internal scale.
type Severity int

// Severity values are ordered from most to least severe to make sorting
// natural (a < b means a is more severe).
const (
	SeverityCritical Severity = iota
	SeverityHigh
	SeverityMedium
	SeverityLow
	SeverityInfo
)

// String returns the canonical lowercase name.
func (s Severity) String() string {
	switch s {
	case SeverityCritical:
		return "critical"
	case SeverityHigh:
		return "high"
	case SeverityMedium:
		return "medium"
	case SeverityLow:
		return "low"
	default:
		return "info"
	}
}

// Finding is one normalized issue extracted from a SARIF document.
type Finding struct {
	// RuleID is the tool's rule identifier (e.g., "CKV_AWS_19").
	RuleID string
	// Severity is the normalized severity bucket.
	Severity Severity
	// Message is the human-readable finding description. We prefer the
	// rule's shortDescription (when SARIF includes it — most tools do)
	// because tools like trivy stuff a multi-line envelope of metadata
	// into result.message.text instead of a short description.
	Message string
	// File is the file path (relative to scanned root) the finding refers to.
	File string
	// Line is the 1-based line number; 0 if unknown.
	Line int
	// Tool is the producer's driver name (e.g., "checkov", "trivy", "kics").
	Tool string
	// HelpURI is the per-rule remediation link from SARIF's
	// tool.driver.rules[].helpUri, when present. Renderers turn the
	// rule ID into a clickable link to this URL so users can jump to
	// the official guideline page directly from the summary.
	HelpURI string
}

// Findings is a parsed SARIF document.
type Findings struct {
	// Tool is the driver name from the first run (most SARIF docs have one run).
	Tool string
	// Findings is the flattened list across all runs in the document.
	Findings []Finding
}

// Count returns the total number of findings.
func (f *Findings) Count() int {
	if f == nil {
		return 0
	}
	return len(f.Findings)
}

// CountsBySeverity returns a map of severity name → count. Keys are the
// canonical lowercase severity names ("critical", "high", "medium", "low",
// "info"). Severities with zero findings are omitted.
func (f *Findings) CountsBySeverity() map[string]int {
	if f == nil {
		return nil
	}
	out := make(map[string]int)
	for _, fd := range f.Findings {
		out[fd.Severity.String()]++
	}
	return out
}

// HighestSeverity returns the most-severe Severity present, or SeverityInfo
// if there are no findings (so callers can treat "no findings" as info-level).
func (f *Findings) HighestSeverity() Severity {
	if f == nil || len(f.Findings) == 0 {
		return SeverityInfo
	}
	highest := SeverityInfo
	for _, fd := range f.Findings {
		if fd.Severity < highest {
			highest = fd.Severity
		}
	}
	return highest
}

// SortedBySeverity returns a copy of Findings ordered most-severe first.
// Ties are broken by rule ID, then by file:line for stable output.
func (f *Findings) SortedBySeverity() []Finding {
	if f == nil {
		return nil
	}
	out := append([]Finding(nil), f.Findings...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity < out[j].Severity
		}
		if out[i].RuleID != out[j].RuleID {
			return out[i].RuleID < out[j].RuleID
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// rawSARIF is the minimal subset of the SARIF 2.1.0 schema this parser cares
// about. Fields we don't need are deliberately omitted; encoding/json drops
// unknown fields by default.
type rawSARIF struct {
	Runs []rawRun `json:"runs"`
}

type rawRun struct {
	Tool    rawTool     `json:"tool"`
	Results []rawResult `json:"results"`
}

type rawTool struct {
	Driver rawDriver `json:"driver"`
}

type rawDriver struct {
	Name  string    `json:"name"`
	Rules []rawRule `json:"rules"`
}

type rawRule struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	ShortDescription     rawMessage       `json:"shortDescription"`
	FullDescription      rawMessage       `json:"fullDescription"`
	HelpURI              string           `json:"helpUri"`
	DefaultConfiguration rawDefaultConfig `json:"defaultConfiguration"`
	Properties           map[string]any   `json:"properties"`
}

type rawDefaultConfig struct {
	Level string `json:"level"`
}

type rawResult struct {
	RuleID     string         `json:"ruleId"`
	Level      string         `json:"level"`
	Message    rawMessage     `json:"message"`
	Locations  []rawLocation  `json:"locations"`
	Properties map[string]any `json:"properties"`
}

type rawMessage struct {
	Text string `json:"text"`
}

type rawLocation struct {
	PhysicalLocation rawPhysicalLocation `json:"physicalLocation"`
}

type rawPhysicalLocation struct {
	ArtifactLocation rawArtifactLocation `json:"artifactLocation"`
	Region           rawRegion           `json:"region"`
}

type rawArtifactLocation struct {
	URI string `json:"uri"`
}

type rawRegion struct {
	StartLine int `json:"startLine"`
}

// Parse decodes a SARIF document and returns normalized Findings.
// Empty input or zero-finding documents yield a non-nil Findings with
// Tool="" and Findings=nil (callers should check Count()).
func Parse(data []byte) (*Findings, error) {
	if len(data) == 0 {
		return &Findings{}, nil
	}
	var raw rawSARIF
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%w: invalid SARIF document: %w", errUtils.ErrParseFile, err)
	}

	out := &Findings{}
	if len(raw.Runs) > 0 {
		out.Tool = raw.Runs[0].Tool.Driver.Name
	}

	for _, run := range raw.Runs {
		driverName := run.Tool.Driver.Name
		// Index rules once per run so result-time lookups (level, message,
		// helpUri) are O(1) rather than O(rules) per result.
		rulesByID := make(map[string]rawRule, len(run.Tool.Driver.Rules))
		for _, rule := range run.Tool.Driver.Rules {
			rulesByID[rule.ID] = rule
		}

		for i := range run.Results {
			res := &run.Results[i]
			rule := rulesByID[res.RuleID] // zero value when missing

			level := res.Level
			if level == "" {
				level = rule.DefaultConfiguration.Level
			}

			severity := classifySeverity(level, res.Properties, rule.Properties)
			file, line := primaryLocation(res.Locations)

			out.Findings = append(out.Findings, Finding{
				RuleID:   res.RuleID,
				Severity: severity,
				Message:  bestMessage(res, &rule),
				File:     file,
				Line:     line,
				Tool:     driverName,
				HelpURI:  rule.HelpURI,
			})
		}
	}

	return out, nil
}

// bestMessage picks the most useful short description for a finding.
// Preference order:
//
//  1. Rule shortDescription — short, intended for table display.
//     (checkov / trivy / kics all populate this with a one-liner.)
//  2. Result message.text first line — covers tools that don't ship
//     rule definitions inline.
//  3. Rule fullDescription first line — last-resort fallback.
//
// Trivy's result.message.text is a multi-line "Artifact / Type /
// Vulnerability / Severity / Message / Link" envelope — useful for a
// human reading SARIF directly, but unsuitable for a one-cell table
// display. For Trivy we want shortDescription; it's the reliable short
// form for all four scanners we ship as built-in kinds.
func bestMessage(res *rawResult, rule *rawRule) string {
	if s := strings.TrimSpace(rule.ShortDescription.Text); s != "" {
		return s
	}
	if s := strings.TrimSpace(res.Message.Text); s != "" {
		return firstLine(s)
	}
	if s := strings.TrimSpace(rule.FullDescription.Text); s != "" {
		return firstLine(s)
	}
	return ""
}

// firstLine returns the first non-empty line of s, trimmed. Used when
// the only available message has multi-line content but we need a
// single-cell display value.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

// classifySeverity maps SARIF level + tool-specific properties to the
// internal Severity bucket. Order:
//  1. Explicit "security-severity" numeric (GitHub convention; some tools emit this).
//  2. Properties.severity string (checkov / trivy / kics conventions).
//  3. SARIF level field ("error"=high, "warning"=medium, "note"/"none"=low/info).
func classifySeverity(level string, resultProps, ruleProps map[string]any) Severity {
	if s, ok := numericSecuritySeverity(resultProps, ruleProps); ok {
		return s
	}
	if s, ok := propertySeverity(resultProps, ruleProps); ok {
		return s
	}
	return levelToSeverity(level)
}

// CVSS-style numeric thresholds used when SARIF carries the GitHub
// "security-severity" convention (a 0.0–10.0 score). Splitting these
// out of the function keeps the body small for the cyclomatic check and
// gives the magic-number rule a name to point at.
const (
	severityCriticalCutoff = 9.0
	severityHighCutoff     = 7.0
	severityMediumCutoff   = 4.0
)

func numericSecuritySeverity(props ...map[string]any) (Severity, bool) {
	for _, p := range props {
		v, ok := parseSecuritySeverityValue(p)
		if !ok {
			continue
		}
		return bucketBySecuritySeverity(v), true
	}
	return SeverityInfo, false
}

// parseSecuritySeverityValue pulls the security-severity field from a
// SARIF properties map, accepting both number and string forms.
func parseSecuritySeverityValue(p map[string]any) (float64, bool) {
	if p == nil {
		return 0, false
	}
	raw, ok := p["security-severity"]
	if !ok {
		return 0, false
	}
	switch t := raw.(type) {
	case float64:
		return t, true
	case string:
		var v float64
		if _, err := fmt.Sscanf(t, "%f", &v); err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// bucketBySecuritySeverity maps a 0.0–10.0 GitHub security-severity
// score to our internal Severity scale.
func bucketBySecuritySeverity(v float64) Severity {
	switch {
	case v >= severityCriticalCutoff:
		return SeverityCritical
	case v >= severityHighCutoff:
		return SeverityHigh
	case v >= severityMediumCutoff:
		return SeverityMedium
	case v > 0:
		return SeverityLow
	default:
		return SeverityInfo
	}
}

func propertySeverity(props ...map[string]any) (Severity, bool) {
	for _, p := range props {
		if p == nil {
			continue
		}
		raw, ok := p["severity"]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		switch strings.ToLower(s) {
		case "critical":
			return SeverityCritical, true
		case "high":
			return SeverityHigh, true
		case "medium", "moderate":
			return SeverityMedium, true
		case "low":
			return SeverityLow, true
		case "info", "informational", "note":
			return SeverityInfo, true
		}
	}
	return SeverityInfo, false
}

func levelToSeverity(level string) Severity {
	switch strings.ToLower(level) {
	case "error":
		return SeverityHigh
	case "warning":
		return SeverityMedium
	case "note":
		return SeverityLow
	case "none", "":
		return SeverityInfo
	default:
		return SeverityInfo
	}
}

func primaryLocation(locs []rawLocation) (string, int) {
	if len(locs) == 0 {
		return "", 0
	}
	loc := locs[0].PhysicalLocation
	return loc.ArtifactLocation.URI, loc.Region.StartLine
}
