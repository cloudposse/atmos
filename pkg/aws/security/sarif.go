//nolint:revive // SARIF model and renderer types stay together to mirror the standard schema.
package security

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// SARIF schema constants. SARIF 2.1.0 is the published OASIS standard
// (https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) and the
// version consumed by GitHub code scanning, Azure DevOps, and most SARIF viewers.
const (
	sarifVersion = "2.1.0"
	// SARIF schema is the SchemaStore canonical mirror of the SARIF 2.1.0
	// JSON Schema. SchemaStore is the stable URL used by editors and
	// validators; the upstream `oasis-tcs/sarif-spec` GitHub layout has
	// shifted branches and directories more than once, so we don't rely on it.
	sarifSchema       = "https://json.schemastore.org/sarif-2.1.0.json"
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
	cweTaxonomyID = "CWE"
)

// SARIFLog is the top-level SARIF document.
type SARIFLog struct {
	Schema  string `json:"$schema,omitempty"`
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

// Run captures the output of a single tool invocation.
type Run struct {
	Tool               Tool                        `json:"tool"`
	Invocations        []Invocation                `json:"invocations,omitempty"`
	OriginalURIBaseIDs map[string]ArtifactLocation `json:"originalUriBaseIds,omitempty"`
	Taxonomies         []ToolComponent             `json:"taxonomies,omitempty"`
	Results            []Result                    `json:"results"`
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

// ToolComponent describes a SARIF tool extension or taxonomy.
type ToolComponent struct {
	Name           string                `json:"name"`
	Version        string                `json:"version,omitempty"`
	InformationURI string                `json:"informationUri,omitempty"`
	GUID           string                `json:"guid,omitempty"`
	Taxa           []ReportingDescriptor `json:"taxa,omitempty"`
}

// ReportingDescriptor describes a rule or taxonomy item.
type ReportingDescriptor struct {
	ID               string           `json:"id"`
	Name             string           `json:"name,omitempty"`
	ShortDescription *MultiformatText `json:"shortDescription,omitempty"`
	FullDescription  *MultiformatText `json:"fullDescription,omitempty"`
}

// Rule describes a class of finding (one entry per unique finding title).
type Rule struct {
	ID               string           `json:"id"`
	Name             string           `json:"name,omitempty"`
	ShortDescription *MultiformatText `json:"shortDescription,omitempty"`
	FullDescription  *MultiformatText `json:"fullDescription,omitempty"`
	Help             *MultiformatText `json:"help,omitempty"`
	HelpURI          string           `json:"helpUri,omitempty"`
	DefaultConfig    *RuleConfig      `json:"defaultConfiguration,omitempty"`
	Relationships    []Relationship   `json:"relationships,omitempty"`
	Properties       map[string]any   `json:"properties,omitempty"`
}

// Relationship links a rule to a taxonomy item.
type Relationship struct {
	Target ReportingDescriptorReference `json:"target"`
	Kinds  []string                     `json:"kinds,omitempty"`
}

// ReportingDescriptorReference references a rule or taxonomy descriptor.
type ReportingDescriptorReference struct {
	ID            string                  `json:"id,omitempty"`
	ToolComponent *ToolComponentReference `json:"toolComponent,omitempty"`
}

// ToolComponentReference references a SARIF tool component by run-level index.
type ToolComponentReference struct {
	Index int `json:"index"`
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
	RuleID          string                         `json:"ruleId"`
	RuleIndex       *int                           `json:"ruleIndex,omitempty"`
	Level           string                         `json:"level,omitempty"`
	Kind            string                         `json:"kind,omitempty"`
	Message         MultiformatText                `json:"message"`
	Locations       []Location                     `json:"locations,omitempty"`
	Taxa            []ReportingDescriptorReference `json:"taxa,omitempty"`
	HostedViewerURI string                         `json:"hostedViewerUri,omitempty"`
	Fingerprints    map[string]string              `json:"fingerprints,omitempty"`
	Properties      map[string]any                 `json:"properties,omitempty"`
}

// Invocation records how the CLI was executed for audit evidence.
type Invocation struct {
	CommandLine         string            `json:"commandLine,omitempty"`
	Arguments           []string          `json:"arguments,omitempty"`
	StartTimeUTC        string            `json:"startTimeUtc,omitempty"`
	EndTimeUTC          string            `json:"endTimeUtc,omitempty"`
	ExitCode            int               `json:"exitCode"`
	ExitCodeDescription string            `json:"exitCodeDescription,omitempty"`
	WorkingDirectory    *ArtifactLocation `json:"workingDirectory,omitempty"`
	ExecutionSuccessful bool              `json:"executionSuccessful"`
	Properties          map[string]any    `json:"properties,omitempty"`
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
	if err := enc.Encode(log); err != nil {
		return fmt.Errorf("%w: failed to encode SARIF log: %w", errUtils.ErrEncode, err)
	}
	return nil
}

// RenderComplianceReport implements ReportRenderer. Compliance failures are
// rendered as SARIF results with a per-control rule.
func (r *sarifRenderer) RenderComplianceReport(w io.Writer, report *ComplianceReport) error {
	defer perf.Track(nil, "security.sarifRenderer.RenderComplianceReport")()

	log := buildComplianceSARIFLog(report)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(log); err != nil {
		return fmt.Errorf("%w: failed to encode SARIF compliance log: %w", errUtils.ErrEncode, err)
	}
	return nil
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
	taxonomies, taxonomyIndex := buildSARIFTaxonomies(findings)
	rules, ruleIndex := buildSARIFRules(findings, taxonomyIndex)
	results := make([]Result, 0, len(findings))
	for i := range findings {
		results = append(results, buildSARIFResult(&findings[i], ruleIndex, taxonomyIndex))
	}
	run := buildSARIFRun(rules, results, taxonomies, report)

	return &SARIFLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs:    []Run{run},
	}
}

// emptySARIFLog returns a well-formed SARIF document with no results.
func emptySARIFLog() *SARIFLog {
	run := buildSARIFRun(nil, []Result{}, nil, nil)
	return &SARIFLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs:    []Run{run},
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
func buildSARIFRules(findings []Finding, taxonomyIndex map[string]int) ([]Rule, map[string]int) {
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
			Help:            ruleHelp(&findings[i]),
			DefaultConfig: &RuleConfig{
				Level: severityToLevel(findings[i].Severity),
			},
			Relationships: ruleRelationships(&findings[i], taxonomyIndex),
			Properties:    ruleProperties(&findings[i]),
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

func ruleHelp(f *Finding) *MultiformatText {
	if f.SourceRemediation == nil || f.SourceRemediation.Text == "" {
		return nil
	}
	help := f.SourceRemediation.Text
	if f.SourceRemediation.URL != "" {
		help += "\n\n" + f.SourceRemediation.URL
	}
	return &MultiformatText{
		Text:     help,
		Markdown: help,
	}
}

func ruleRelationships(f *Finding, taxonomyIndex map[string]int) []Relationship {
	if len(taxonomyIndex) == 0 {
		return nil
	}
	relationships := make([]Relationship, 0)
	for _, standard := range findingComplianceStandards(f) {
		if standard.ID == "" {
			continue
		}
		idx, ok := taxonomyIndex[taxonomyKey(standard.ID)]
		if !ok {
			continue
		}
		targetID := f.SecurityControlID
		if targetID == "" {
			targetID = standard.ID
		}
		relationships = append(relationships, Relationship{
			Target: ReportingDescriptorReference{
				ID:            targetID,
				ToolComponent: &ToolComponentReference{Index: idx},
			},
			Kinds: []string{"superset"},
		})
	}
	return relationships
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

// buildSARIFResult emits a single Result for a finding.
func buildSARIFResult(f *Finding, ruleIndex map[string]int, taxonomyIndex map[string]int) Result {
	key := ruleKey(f)
	idx, ok := ruleIndex[key]
	res := Result{
		RuleID:     key,
		Level:      severityToLevel(f.Severity),
		Kind:       sarifKindFail,
		Message:    resultMessage(f),
		Properties: resultProperties(f),
	}
	if ok {
		res.RuleIndex = &idx
	}
	res.Locations = buildLocations(f)
	res.Taxa = resultTaxa(f, taxonomyIndex)
	res.HostedViewerURI = f.SourceURL
	if f.ID != "" {
		res.Fingerprints = map[string]string{"atmos/v1": f.ID}
	}
	return res
}

// resultMessage prefers the finding description, falling back to the title.
func resultMessage(f *Finding) MultiformatText {
	text := ""
	switch {
	case f.Description != "":
		text = f.Description
	case f.Title != "":
		text = f.Title
	default:
		text = string(f.Source)
	}
	if f.SourceRemediation == nil || f.SourceRemediation.Text == "" {
		return MultiformatText{Text: text}
	}
	markdown := text + "\n\n### Remediation\n\n" + f.SourceRemediation.Text
	if f.SourceRemediation.URL != "" {
		markdown += "\n\n" + f.SourceRemediation.URL
	}
	return MultiformatText{Text: text, Markdown: markdown}
}

// resultProperties carries Atmos-specific metadata (mapping, remediation) so
// downstream tooling can render the same context the Markdown renderer shows.
//
//nolint:gocognit,cyclop,funlen // SARIF properties intentionally gather several independent optional source fields.
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
	addNonEmpty(props, "source_url", f.SourceURL)

	if f.SourceSeverity != nil {
		if f.SourceSeverity.Score != nil {
			props["security-severity"] = formatSecuritySeverity(*f.SourceSeverity.Score)
		}
		addNonEmpty(props, "source_severity_label", f.SourceSeverity.Label)
	}
	if f.SourceLifecycle != nil {
		if lifecycle := compactStringMap(map[string]string{
			"workflow_status":   f.SourceLifecycle.WorkflowStatus,
			"record_state":      f.SourceLifecycle.RecordState,
			"compliance_status": f.SourceLifecycle.ComplianceStatus,
			"inspector_status":  f.SourceLifecycle.InspectorStatus,
		}); lifecycle != nil {
			props["source_lifecycle"] = lifecycle
		}
	}
	if f.SourceTimestamps != nil {
		if timestamps := compactStringMap(map[string]string{
			"first_observed_at": formatSARIFTime(f.SourceTimestamps.FirstObservedAt),
			"last_observed_at":  formatSARIFTime(f.SourceTimestamps.LastObservedAt),
			"updated_at":        formatSARIFTime(f.SourceTimestamps.UpdatedAt),
			"created_at":        formatSARIFTime(f.SourceTimestamps.CreatedAt),
		}); timestamps != nil {
			props["source_timestamps"] = timestamps
		}
	}
	if f.SourceRemediation != nil {
		addNonEmpty(props, "source_remediation_text", f.SourceRemediation.Text)
		addNonEmpty(props, "remediation_url", f.SourceRemediation.URL)
	}
	if f.Vulnerability != nil {
		props["vulnerability"] = vulnerabilityProperties(f.Vulnerability)
	}
	if standards := complianceStandardsProperties(f); len(standards) > 0 {
		props["compliance_standards"] = standards
	}

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

func formatSecuritySeverity(score float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", score), "0"), ".")
}

func compactStringMap(values map[string]string) map[string]string {
	out := make(map[string]string)
	for key, value := range values {
		if value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func formatSARIFTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func vulnerabilityProperties(v *VulnerabilityDetails) map[string]any {
	props := make(map[string]any)
	addNonEmpty(props, "id", v.ID)
	addNonEmpty(props, "cve_id", v.CVEID)
	addNonEmpty(props, "package_name", v.PackageName)
	addNonEmpty(props, "package_version", v.PackageVersion)
	addNonEmpty(props, "fixed_in_version", v.FixedInVersion)
	if v.EPSSScore > 0 {
		props["epss_score"] = v.EPSSScore
	}
	if len(v.CWEIDs) > 0 {
		props["cwe_ids"] = v.CWEIDs
	}
	if len(v.Packages) > 0 {
		props["packages"] = v.Packages
	}
	if len(v.ReferenceURLs) > 0 {
		props["reference_urls"] = v.ReferenceURLs
	}
	if len(v.CVSS) > 0 {
		props["cvss"] = v.CVSS
	}
	return props
}

func complianceStandardsProperties(f *Finding) []map[string]string {
	standards := findingComplianceStandards(f)
	if len(standards) == 0 {
		return nil
	}
	out := make([]map[string]string, 0, len(standards))
	for _, standard := range standards {
		props := compactStringMap(map[string]string{
			"id":      standard.ID,
			"name":    standard.Name,
			"version": standard.Version,
		})
		if props != nil {
			out = append(out, props)
		}
	}
	return out
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
					URI:       normalizeArtifactURI(f.Mapping.ComponentPath),
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

func normalizeArtifactURI(path string) string {
	if path == "" {
		return ""
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		if wd, err := os.Getwd(); err == nil {
			if rel, relErr := filepath.Rel(wd, clean); relErr == nil && !strings.HasPrefix(rel, "..") {
				clean = rel
			}
		}
	}
	return filepath.ToSlash(clean)
}

func resultTaxa(f *Finding, taxonomyIndex map[string]int) []ReportingDescriptorReference {
	if f.Vulnerability == nil || len(f.Vulnerability.CWEIDs) == 0 {
		return nil
	}
	idx, ok := taxonomyIndex[taxonomyKey(cweTaxonomyID)]
	if !ok {
		return nil
	}
	taxa := make([]ReportingDescriptorReference, 0, len(f.Vulnerability.CWEIDs))
	for _, cwe := range f.Vulnerability.CWEIDs {
		taxa = append(taxa, ReportingDescriptorReference{
			ID:            cwe,
			ToolComponent: &ToolComponentReference{Index: idx},
		})
	}
	return taxa
}

//nolint:gocognit,cyclop,funlen // Taxonomy construction has separate compliance and CWE passes for deterministic SARIF output.
func buildSARIFTaxonomies(findings []Finding) ([]ToolComponent, map[string]int) {
	compliance := make(map[string]map[string]string)
	cweIDs := make(map[string]struct{})
	for i := range findings {
		finding := &findings[i]
		for _, standard := range findingComplianceStandards(finding) {
			if standard.ID == "" {
				continue
			}
			key := taxonomyKey(standard.ID)
			if _, ok := compliance[key]; !ok {
				compliance[key] = make(map[string]string)
			}
			controlID := finding.SecurityControlID
			if controlID == "" {
				controlID = standard.ID
			}
			controlName := finding.Title
			if controlName == "" {
				controlName = controlID
			}
			compliance[key][controlID] = controlName
		}
		if finding.Vulnerability != nil {
			for _, cwe := range finding.Vulnerability.CWEIDs {
				cweIDs[cwe] = struct{}{}
			}
		}
	}

	taxonomies := make([]ToolComponent, 0, len(compliance)+1)
	keys := make([]string, 0, len(compliance))
	for key := range compliance {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		standardID := strings.TrimPrefix(key, "compliance:")
		standard := parseComplianceStandard(standardID)
		taxa := make([]ReportingDescriptor, 0, len(compliance[key]))
		controlIDs := make([]string, 0, len(compliance[key]))
		for controlID := range compliance[key] {
			controlIDs = append(controlIDs, controlID)
		}
		sort.Strings(controlIDs)
		for _, controlID := range controlIDs {
			taxa = append(taxa, ReportingDescriptor{
				ID:   controlID,
				Name: compliance[key][controlID],
				ShortDescription: &MultiformatText{
					Text: compliance[key][controlID],
				},
			})
		}
		taxonomies = append(taxonomies, ToolComponent{
			Name:           complianceTaxonomyName(standard),
			Version:        standard.Version,
			InformationURI: complianceInformationURI(standard.ID),
			GUID:           stableTaxonomyGUID(standard.ID),
			Taxa:           taxa,
		})
	}

	if len(cweIDs) > 0 {
		ids := make([]string, 0, len(cweIDs))
		for id := range cweIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		taxa := make([]ReportingDescriptor, 0, len(ids))
		for _, id := range ids {
			taxa = append(taxa, ReportingDescriptor{ID: id, Name: id})
		}
		taxonomies = append(taxonomies, ToolComponent{
			Name:           cweTaxonomyID,
			InformationURI: "https://cwe.mitre.org/",
			GUID:           stableTaxonomyGUID(cweTaxonomyID),
			Taxa:           taxa,
		})
	}

	index := make(map[string]int, len(taxonomies))
	for i, key := range keys {
		index[key] = i
	}
	if len(cweIDs) > 0 {
		index[taxonomyKey(cweTaxonomyID)] = len(taxonomies) - 1
	}
	return taxonomies, index
}

func findingComplianceStandards(f *Finding) []ComplianceStandard {
	if f == nil {
		return nil
	}
	if len(f.ComplianceStandards) > 0 {
		return f.ComplianceStandards
	}
	if f.ComplianceStandard == "" {
		return nil
	}
	return []ComplianceStandard{parseComplianceStandard(f.ComplianceStandard)}
}

func taxonomyKey(id string) string {
	if id == cweTaxonomyID {
		return "cwe"
	}
	return "compliance:" + id
}

func complianceTaxonomyName(standard ComplianceStandard) string {
	name := standard.Name
	if name == "" {
		name = standard.ID
	}
	if standard.Version != "" {
		return name + " v" + standard.Version
	}
	return name
}

func complianceInformationURI(id string) string {
	switch {
	case strings.Contains(id, "cis-aws-foundations-benchmark"):
		return "https://www.cisecurity.org/benchmark/amazon_web_services"
	case strings.Contains(id, "aws-foundational-security-best-practices"):
		return "https://docs.aws.amazon.com/securityhub/latest/userguide/fsbp-standard.html"
	}
	return ""
}

func stableTaxonomyGUID(name string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://atmos.tools/sarif/taxonomy/"+name)).String()
}

func buildSARIFRun(rules []Rule, results []Result, taxonomies []ToolComponent, report *Report) Run {
	run := Run{
		Tool: Tool{
			Driver: Driver{
				Name:            sarifToolName,
				Version:         version.Version,
				SemanticVersion: version.Version,
				InformationURI:  sarifInfoURI,
				Rules:           rules,
			},
		},
		Taxonomies: taxonomies,
		Results:    results,
	}
	if hasPhysicalLocations(results) {
		rootURI := "file:///"
		if wd, err := os.Getwd(); err == nil {
			rootURI = fileDirectoryURI(wd)
		}
		run.OriginalURIBaseIDs = map[string]ArtifactLocation{
			"%SRCROOT%": {URI: rootURI},
		}
	}
	if report != nil && report.Invocation != nil {
		run.Invocations = []Invocation{buildSARIFInvocation(report.Invocation)}
	}
	return run
}

func hasPhysicalLocations(results []Result) bool {
	for i := range results {
		for _, location := range results[i].Locations {
			if location.PhysicalLocation != nil && location.PhysicalLocation.ArtifactLocation != nil {
				return true
			}
		}
	}
	return false
}

func buildSARIFInvocation(inv *ReportInvocation) Invocation {
	out := Invocation{
		CommandLine:         inv.CommandLine,
		Arguments:           inv.Arguments,
		StartTimeUTC:        inv.StartTimeUTC.UTC().Format(time.RFC3339),
		EndTimeUTC:          inv.EndTimeUTC.UTC().Format(time.RFC3339),
		ExitCode:            inv.ExitCode,
		ExitCodeDescription: inv.ExitCodeDescription,
		ExecutionSuccessful: inv.ExecutionSuccessful,
		Properties: map[string]any{
			"accounts_scanned": inv.AccountsScanned,
			"regions_scanned":  inv.RegionsScanned,
			"stacks_scanned":   inv.StacksScanned,
		},
	}
	if inv.WorkingDirectory != "" {
		out.WorkingDirectory = &ArtifactLocation{URI: fileDirectoryURI(inv.WorkingDirectory)}
	}
	if out.ExitCodeDescription == "" && out.ExecutionSuccessful {
		out.ExitCodeDescription = "Success"
	}
	return out
}

func fileDirectoryURI(path string) string {
	uriPath := filepath.ToSlash(path)
	if !strings.HasSuffix(uriPath, "/") {
		uriPath += "/"
	}
	return "file://" + uriPath
}

// wrapSARIFLog wraps the rules + results into a complete SARIF log document.
// A nil results slice is normalized to an empty slice so the JSON output
// always emits `"results": []` rather than `"results": null` — SARIF
// consumers (GitHub code scanning in particular) reject null arrays.
func wrapSARIFLog(rules []Rule, results []Result) *SARIFLog {
	if results == nil {
		results = []Result{}
	}
	run := buildSARIFRun(rules, results, nil, nil)
	return &SARIFLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs:    []Run{run},
	}
}
