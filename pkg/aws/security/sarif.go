package security

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// SARIF schema constants. SARIF 2.1.0 is the published OASIS standard
// (https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) and the
// version consumed by GitHub code scanning, Azure DevOps, and most SARIF viewers.
const (
	sarifVersion      = "2.1.0"
	sarifSchema       = "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json"
	sarifToolName     = "atmos"
	sarifInfoURI      = "https://atmos.tools"
	sarifLevelError   = "error"
	sarifLevelWarning = "warning"
	sarifLevelNote    = "note"
	sarifLevelNone    = "none"
	sarifKindFail     = "fail"

	// Common property keys repeated across rules and results.
	propSeverity  = "severity"
	propFramework = "framework"
)

// SARIFLog is the top-level SARIF document.
type SARIFLog struct {
	Schema  string `json:"$schema,omitempty"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

// Run captures the output of a single tool invocation.
type Run struct {
	Tool    Tool     `json:"tool"`
	Results []Result `json:"results"`
}

// Tool describes the analysis tool that produced the run.
type Tool struct {
	Driver Driver `json:"driver"`
}

// Driver is the primary analysis tool.
type Driver struct {
	Name            string `json:"name"`
	Version         string `json:"version,omitempty"`
	SemanticVersion string `json:"semanticVersion,omitempty"`
	InformationURI  string `json:"informationUri,omitempty"`
	Rules           []Rule `json:"rules,omitempty"`
}

// Rule describes a class of finding (one entry per unique finding title).
type Rule struct {
	ID               string           `json:"id"`
	Name             string           `json:"name,omitempty"`
	ShortDescription *MultiformatText `json:"shortDescription,omitempty"`
	FullDescription  *MultiformatText `json:"fullDescription,omitempty"`
	HelpURI          string           `json:"helpUri,omitempty"`
	DefaultConfig    *RuleConfig      `json:"defaultConfiguration,omitempty"`
	Properties       map[string]any   `json:"properties,omitempty"`
}

// RuleConfig sets per-rule defaults such as level.
type RuleConfig struct {
	Level string `json:"level,omitempty"`
}

// MultiformatText is a SARIF message/description container.
type MultiformatText struct {
	Text     string `json:"text,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}

// Result is a single finding occurrence.
type Result struct {
	RuleID       string            `json:"ruleId"`
	RuleIndex    *int              `json:"ruleIndex,omitempty"`
	Level        string            `json:"level,omitempty"`
	Kind         string            `json:"kind,omitempty"`
	Message      MultiformatText   `json:"message"`
	Locations    []Location        `json:"locations,omitempty"`
	Fingerprints map[string]string `json:"fingerprints,omitempty"`
	Properties   map[string]any    `json:"properties,omitempty"`
}

// Location identifies where a result occurred.
type Location struct {
	PhysicalLocation *PhysicalLocation `json:"physicalLocation,omitempty"`
	LogicalLocations []LogicalLocation `json:"logicalLocations,omitempty"`
	Message          *MultiformatText  `json:"message,omitempty"`
}

// PhysicalLocation points at a file (and optional region).
type PhysicalLocation struct {
	ArtifactLocation *ArtifactLocation `json:"artifactLocation,omitempty"`
	Region           *Region           `json:"region,omitempty"`
}

// ArtifactLocation is a URI reference for a file in the repository.
type ArtifactLocation struct {
	URI       string `json:"uri,omitempty"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

// Region is a sub-range within an artifact.
type Region struct {
	StartLine int `json:"startLine,omitempty"`
}

// LogicalLocation references a non-file entity (e.g., an AWS resource ARN).
type LogicalLocation struct {
	Name               string `json:"name,omitempty"`
	FullyQualifiedName string `json:"fullyQualifiedName,omitempty"`
	Kind               string `json:"kind,omitempty"`
}

// sarifRenderer renders security/compliance reports as SARIF 2.1.0 JSON.
type sarifRenderer struct{}

// RenderSecurityReport implements ReportRenderer.
func (r *sarifRenderer) RenderSecurityReport(w io.Writer, report *Report) error {
	defer perf.Track(nil, "security.sarifRenderer.RenderSecurityReport")()

	log := BuildSARIFLog(report)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

// RenderComplianceReport implements ReportRenderer. Compliance failures are
// rendered as SARIF results with a per-control rule.
func (r *sarifRenderer) RenderComplianceReport(w io.Writer, report *ComplianceReport) error {
	defer perf.Track(nil, "security.sarifRenderer.RenderComplianceReport")()

	log := buildComplianceSARIFLog(report)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(log)
}

// BuildSARIFLog converts a security Report into a SARIF 2.1.0 log.
// The mapping is stable: rules are derived from unique finding titles, results
// are emitted in a deterministic order so output is reproducible across runs.
func BuildSARIFLog(report *Report) *SARIFLog {
	defer perf.Track(nil, "security.BuildSARIFLog")()

	if report == nil {
		return emptySARIFLog()
	}

	findings := sortedFindings(report.Findings)
	rules, ruleIndex := buildSARIFRules(findings)
	results := make([]Result, 0, len(findings))
	for i := range findings {
		results = append(results, buildSARIFResult(&findings[i], ruleIndex))
	}

	return &SARIFLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []Run{
			{
				Tool: Tool{
					Driver: Driver{
						Name:            sarifToolName,
						Version:         version.Version,
						SemanticVersion: version.Version,
						InformationURI:  sarifInfoURI,
						Rules:           rules,
					},
				},
				Results: results,
			},
		},
	}
}

// emptySARIFLog returns a well-formed SARIF document with no results.
func emptySARIFLog() *SARIFLog {
	return &SARIFLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []Run{
			{
				Tool: Tool{
					Driver: Driver{
						Name:            sarifToolName,
						Version:         version.Version,
						SemanticVersion: version.Version,
						InformationURI:  sarifInfoURI,
					},
				},
				Results: []Result{},
			},
		},
	}
}

// sortedFindings returns findings sorted by (severity rank desc, ID asc) so
// SARIF output is byte-stable for the same input report.
func sortedFindings(findings []Finding) []Finding {
	out := make([]Finding, len(findings))
	copy(out, findings)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Severity ranks (higher is more severe) used to sort SARIF results
// deterministically: critical findings first, informational last.
const (
	rankCritical      = 5
	rankHigh          = 4
	rankMedium        = 3
	rankLow           = 2
	rankInformational = 1
	rankUnknown       = 0
)

// severityRank maps a Severity to a numeric rank for sorting.
func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return rankCritical
	case SeverityHigh:
		return rankHigh
	case SeverityMedium:
		return rankMedium
	case SeverityLow:
		return rankLow
	case SeverityInformational:
		return rankInformational
	}
	return rankUnknown
}

// buildSARIFRules emits one rule per unique finding title and returns a
// title->index map for result references.
func buildSARIFRules(findings []Finding) ([]Rule, map[string]int) {
	rules := make([]Rule, 0)
	index := make(map[string]int)
	for i := range findings {
		title := findings[i].Title
		if title == "" {
			title = string(findings[i].Source)
		}
		key := ruleKey(&findings[i])
		if _, ok := index[key]; ok {
			continue
		}
		index[key] = len(rules)
		rules = append(rules, Rule{
			ID:   key,
			Name: title,
			ShortDescription: &MultiformatText{
				Text: title,
			},
			FullDescription: ruleFullDescription(&findings[i]),
			DefaultConfig: &RuleConfig{
				Level: severityToLevel(findings[i].Severity),
			},
			Properties: ruleProperties(&findings[i]),
		})
	}
	return rules, index
}

// ruleKey returns a stable identifier for the rule a finding belongs to.
// Prefer the security control ID (e.g., "EC2.18") when available, fall back to
// a slug of the title so duplicate findings collapse to one rule.
func ruleKey(f *Finding) string {
	if f.SecurityControlID != "" {
		return f.SecurityControlID
	}
	if f.Title != "" {
		return slugify(f.Title)
	}
	return fmt.Sprintf("%s/%s", f.Source, f.ID)
}

// slugify lowercases a string and replaces non-alphanumeric runs with '-'.
func slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "finding"
	}
	return out
}

// ruleFullDescription prefers the description, falling back to title.
func ruleFullDescription(f *Finding) *MultiformatText {
	desc := f.Description
	if desc == "" {
		desc = f.Title
	}
	if desc == "" {
		return nil
	}
	return &MultiformatText{Text: desc}
}

// ruleProperties surfaces classification metadata on the rule (consumed by
// GitHub code scanning to render filters and tags).
func ruleProperties(f *Finding) map[string]any {
	tags := make([]string, 0, 4)
	tags = append(tags, "security", string(f.Source))
	if f.ComplianceStandard != "" {
		tags = append(tags, f.ComplianceStandard)
	}
	if f.ResourceType != "" {
		tags = append(tags, f.ResourceType)
	}
	props := map[string]any{
		"tags":       tags,
		propSeverity: string(f.Severity),
	}
	if f.SecurityControlID != "" {
		props["security-severity"] = severityToSecuritySeverity(f.Severity)
		props["control-id"] = f.SecurityControlID
	}
	return props
}

// severityToLevel maps an Atmos severity to a SARIF result level.
// CRITICAL/HIGH escalate to "error", MEDIUM to "warning", LOW/INFORMATIONAL to
// "note". This is the same mapping GitHub code scanning uses for filtering.
func severityToLevel(s Severity) string {
	switch s {
	case SeverityCritical, SeverityHigh:
		return sarifLevelError
	case SeverityMedium:
		return sarifLevelWarning
	case SeverityLow, SeverityInformational:
		return sarifLevelNote
	}
	return sarifLevelNone
}

// severityToSecuritySeverity returns a numeric severity in the 0.0-10.0 range
// per the GitHub Advanced Security convention.
func severityToSecuritySeverity(s Severity) string {
	switch s {
	case SeverityCritical:
		return "9.5"
	case SeverityHigh:
		return "8.0"
	case SeverityMedium:
		return "5.5"
	case SeverityLow:
		return "3.0"
	case SeverityInformational:
		return "1.0"
	}
	return "0.0"
}

// buildSARIFResult emits a single Result for a finding.
func buildSARIFResult(f *Finding, ruleIndex map[string]int) Result {
	key := ruleKey(f)
	idx, ok := ruleIndex[key]
	res := Result{
		RuleID:     key,
		Level:      severityToLevel(f.Severity),
		Kind:       sarifKindFail,
		Message:    MultiformatText{Text: resultMessage(f)},
		Properties: resultProperties(f),
	}
	if ok {
		res.RuleIndex = &idx
	}
	res.Locations = buildLocations(f)
	if f.ID != "" {
		res.Fingerprints = map[string]string{"atmos/v1": f.ID}
	}
	return res
}

// resultMessage prefers the finding description, falling back to the title.
func resultMessage(f *Finding) string {
	if f.Description != "" {
		return f.Description
	}
	if f.Title != "" {
		return f.Title
	}
	return string(f.Source)
}

// resultProperties carries Atmos-specific metadata (mapping, remediation) so
// downstream tooling can render the same context the Markdown renderer shows.
func resultProperties(f *Finding) map[string]any {
	props := map[string]any{
		propSeverity: string(f.Severity),
		"source":     string(f.Source),
	}
	addNonEmpty(props, "account_id", f.AccountID)
	addNonEmpty(props, "region", f.Region)
	addNonEmpty(props, "resource_arn", f.ResourceARN)
	addNonEmpty(props, "resource_type", f.ResourceType)
	addNonEmpty(props, "compliance_standard", f.ComplianceStandard)
	addNonEmpty(props, "security_control_id", f.SecurityControlID)

	if len(f.ResourceTags) > 0 {
		props["resource_tags"] = f.ResourceTags
	}

	if f.Mapping != nil {
		props["mapped"] = f.Mapping.Mapped
		addNonEmpty(props, "confidence", string(f.Mapping.Confidence))
		addNonEmpty(props, "mapping_method", f.Mapping.Method)
		addNonEmpty(props, "stack", f.Mapping.Stack)
		addNonEmpty(props, "component", f.Mapping.Component)
		addNonEmpty(props, "component_path", f.Mapping.ComponentPath)
		addNonEmpty(props, "workspace", f.Mapping.Workspace)
	}

	if f.Remediation != nil {
		addNonEmpty(props, "remediation_description", f.Remediation.Description)
		addNonEmpty(props, "remediation_root_cause", f.Remediation.RootCause)
		addNonEmpty(props, "remediation_deploy_command", f.Remediation.DeployCommand)
		addNonEmpty(props, "remediation_risk_level", f.Remediation.RiskLevel)
		if len(f.Remediation.Steps) > 0 {
			props["remediation_steps"] = f.Remediation.Steps
		}
		if len(f.Remediation.References) > 0 {
			props["remediation_references"] = f.Remediation.References
		}
		if len(f.Remediation.CodeChanges) > 0 {
			props["remediation_code_changes"] = f.Remediation.CodeChanges
		}
	}

	return props
}

// addNonEmpty inserts the value only when it's non-empty, keeping JSON output compact.
func addNonEmpty(m map[string]any, key, value string) {
	if value == "" {
		return
	}
	m[key] = value
}

// buildLocations emits a physical location for mapped findings (so code-scanning
// UIs annotate the Terraform component file) and falls back to a logical
// location with the resource ARN for unmapped findings.
func buildLocations(f *Finding) []Location {
	if f.Mapping != nil && f.Mapping.Mapped && f.Mapping.ComponentPath != "" {
		loc := Location{
			PhysicalLocation: &PhysicalLocation{
				ArtifactLocation: &ArtifactLocation{
					URI:       f.Mapping.ComponentPath,
					URIBaseID: "%SRCROOT%",
				},
			},
		}
		if f.ResourceARN != "" {
			loc.LogicalLocations = []LogicalLocation{
				{
					Name:               f.ResourceARN,
					FullyQualifiedName: f.ResourceARN,
					Kind:               "resource",
				},
			}
		}
		return []Location{loc}
	}

	if f.ResourceARN == "" {
		return nil
	}
	return []Location{
		{
			LogicalLocations: []LogicalLocation{
				{
					Name:               f.ResourceARN,
					FullyQualifiedName: f.ResourceARN,
					Kind:               "resource",
				},
			},
		},
	}
}

// wrapSARIFLog wraps the rules + results into a complete SARIF log document.
func wrapSARIFLog(rules []Rule, results []Result) *SARIFLog {
	return &SARIFLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []Run{
			{
				Tool: Tool{
					Driver: Driver{
						Name:            sarifToolName,
						Version:         version.Version,
						SemanticVersion: version.Version,
						InformationURI:  sarifInfoURI,
						Rules:           rules,
					},
				},
				Results: results,
			},
		},
	}
}
