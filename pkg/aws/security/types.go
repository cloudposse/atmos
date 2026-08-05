package security

import (
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Severity represents a security finding severity level.
type Severity string

const (
	SeverityCritical      Severity = "CRITICAL"
	SeverityHigh          Severity = "HIGH"
	SeverityMedium        Severity = "MEDIUM"
	SeverityLow           Severity = "LOW"
	SeverityInformational Severity = "INFORMATIONAL"
)

// Source represents the AWS security service that generated a finding.
type Source string

const (
	SourceSecurityHub    Source = "security-hub"
	SourceConfig         Source = "config"
	SourceInspector      Source = "inspector"
	SourceGuardDuty      Source = "guardduty"
	SourceMacie          Source = "macie"
	SourceAccessAnalyzer Source = "access-analyzer"
	SourceAll            Source = "all"
)

// MappingConfidence represents how confident the finding-to-code mapping is.
type MappingConfidence string

const (
	ConfidenceExact  MappingConfidence = "exact"  // Tag-based (Path A).
	ConfidenceHigh   MappingConfidence = "high"   // Terraform state match.
	ConfidenceMedium MappingConfidence = "medium" // Naming convention match.
	ConfidenceLow    MappingConfidence = "low"    // Resource type + AI inference.
	ConfidenceNone   MappingConfidence = "none"   // No match found.
)

// Finding represents a normalized security finding from any AWS security service.
type Finding struct {
	ID                  string                `json:"id" yaml:"id"`
	Title               string                `json:"title" yaml:"title"`
	Description         string                `json:"description" yaml:"description"`
	Severity            Severity              `json:"severity" yaml:"severity"`
	Source              Source                `json:"source" yaml:"source"`
	SourceSeverity      *SourceSeverity       `json:"source_severity,omitempty" yaml:"source_severity,omitempty"`
	SourceLifecycle     *SourceLifecycle      `json:"source_lifecycle,omitempty" yaml:"source_lifecycle,omitempty"`
	SourceTimestamps    *SourceTimestamps     `json:"source_timestamps,omitempty" yaml:"source_timestamps,omitempty"`
	SourceRemediation   *SourceRemediation    `json:"source_remediation,omitempty" yaml:"source_remediation,omitempty"`
	SourceURL           string                `json:"source_url,omitempty" yaml:"source_url,omitempty"`
	ComplianceStandard  string                `json:"compliance_standard,omitempty" yaml:"compliance_standard,omitempty"`
	ComplianceStandards []ComplianceStandard  `json:"compliance_standards,omitempty" yaml:"compliance_standards,omitempty"`
	SecurityControlID   string                `json:"security_control_id,omitempty" yaml:"security_control_id,omitempty"` // Per-control ID (e.g., "EC2.18") for compliance deduplication.
	ResourceARN         string                `json:"resource_arn" yaml:"resource_arn"`
	ResourceType        string                `json:"resource_type" yaml:"resource_type"`
	ResourceTags        map[string]string     `json:"resource_tags,omitempty" yaml:"resource_tags,omitempty"` // Tags from the Security Hub finding (no extra API call needed).
	AccountID           string                `json:"account_id" yaml:"account_id"`
	Region              string                `json:"region" yaml:"region"`
	CreatedAt           time.Time             `json:"created_at" yaml:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at" yaml:"updated_at"`
	Vulnerability       *VulnerabilityDetails `json:"vulnerability,omitempty" yaml:"vulnerability,omitempty"`
	Mapping             *ComponentMapping     `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	Remediation         *Remediation          `json:"remediation,omitempty" yaml:"remediation,omitempty"`
}

// SourceSeverity preserves raw severity values from AWS source feeds.
type SourceSeverity struct {
	Score *float64 `json:"score,omitempty" yaml:"score,omitempty"`
	Label string   `json:"label,omitempty" yaml:"label,omitempty"`
}

// SourceLifecycle preserves raw lifecycle state from AWS source feeds.
type SourceLifecycle struct {
	WorkflowStatus   string `json:"workflow_status,omitempty" yaml:"workflow_status,omitempty"`
	RecordState      string `json:"record_state,omitempty" yaml:"record_state,omitempty"`
	ComplianceStatus string `json:"compliance_status,omitempty" yaml:"compliance_status,omitempty"`
	InspectorStatus  string `json:"inspector_status,omitempty" yaml:"inspector_status,omitempty"`
}

// SourceTimestamps preserves AWS source-feed timestamps separately from report upload time.
type SourceTimestamps struct {
	FirstObservedAt *time.Time `json:"first_observed_at,omitempty" yaml:"first_observed_at,omitempty"`
	LastObservedAt  *time.Time `json:"last_observed_at,omitempty" yaml:"last_observed_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	CreatedAt       *time.Time `json:"created_at,omitempty" yaml:"created_at,omitempty"`
}

// SourceRemediation preserves remediation guidance supplied by AWS.
type SourceRemediation struct {
	Text string `json:"text,omitempty" yaml:"text,omitempty"`
	URL  string `json:"url,omitempty" yaml:"url,omitempty"`
}

// ComplianceStandard captures a source framework/control reference.
type ComplianceStandard struct {
	ID      string `json:"id,omitempty" yaml:"id,omitempty"`
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// VulnerabilityDetails preserves structured package vulnerability data from AWS.
type VulnerabilityDetails struct {
	ID             string              `json:"id,omitempty" yaml:"id,omitempty"`
	CVEID          string              `json:"cve_id,omitempty" yaml:"cve_id,omitempty"`
	CWEIDs         []string            `json:"cwe_ids,omitempty" yaml:"cwe_ids,omitempty"`
	EPSSScore      float64             `json:"epss_score,omitempty" yaml:"epss_score,omitempty"`
	PackageName    string              `json:"package_name,omitempty" yaml:"package_name,omitempty"`
	PackageVersion string              `json:"package_version,omitempty" yaml:"package_version,omitempty"`
	FixedInVersion string              `json:"fixed_in_version,omitempty" yaml:"fixed_in_version,omitempty"`
	Packages       []VulnerablePackage `json:"packages,omitempty" yaml:"packages,omitempty"`
	ReferenceURLs  []string            `json:"reference_urls,omitempty" yaml:"reference_urls,omitempty"`
	CVSS           []CVSSScore         `json:"cvss,omitempty" yaml:"cvss,omitempty"`
}

// VulnerablePackage captures an affected package and available fix.
type VulnerablePackage struct {
	Name           string `json:"name,omitempty" yaml:"name,omitempty"`
	Version        string `json:"version,omitempty" yaml:"version,omitempty"`
	FixedInVersion string `json:"fixed_in_version,omitempty" yaml:"fixed_in_version,omitempty"`
	PackageManager string `json:"package_manager,omitempty" yaml:"package_manager,omitempty"`
	Remediation    string `json:"remediation,omitempty" yaml:"remediation,omitempty"`
	FilePath       string `json:"file_path,omitempty" yaml:"file_path,omitempty"`
}

// CVSSScore captures source CVSS score details.
type CVSSScore struct {
	BaseScore float64 `json:"base_score,omitempty" yaml:"base_score,omitempty"`
	Vector    string  `json:"vector,omitempty" yaml:"vector,omitempty"`
	Source    string  `json:"source,omitempty" yaml:"source,omitempty"`
	Version   string  `json:"version,omitempty" yaml:"version,omitempty"`
}

// ComponentMapping represents the resolved mapping from a finding to an Atmos component/stack.
type ComponentMapping struct {
	Stack         string            `json:"stack" yaml:"stack"`
	Component     string            `json:"component" yaml:"component"`
	ComponentPath string            `json:"component_path" yaml:"component_path"`
	Workspace     string            `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	Mapped        bool              `json:"mapped" yaml:"mapped"`
	Confidence    MappingConfidence `json:"confidence" yaml:"confidence"`
	Method        string            `json:"method" yaml:"method"` // How the mapping was determined (e.g., "tag", "state", "naming", "ai").
}

// Remediation contains AI-generated remediation details for a finding.
// This is the output contract — every AI provider must populate these fields
// following the same structure, ensuring consistent and reproducible output.
type Remediation struct {
	Description   string       `json:"description" yaml:"description"`                           // Brief summary of the remediation.
	RootCause     string       `json:"root_cause,omitempty" yaml:"root_cause,omitempty"`         // Why this finding exists in the infrastructure.
	Steps         []string     `json:"steps,omitempty" yaml:"steps,omitempty"`                   // Ordered remediation steps.
	CodeChanges   []CodeChange `json:"code_changes,omitempty" yaml:"code_changes,omitempty"`     // Specific Terraform/HCL changes.
	StackChanges  string       `json:"stack_changes,omitempty" yaml:"stack_changes,omitempty"`   // Specific stack YAML changes.
	DeployCommand string       `json:"deploy_command,omitempty" yaml:"deploy_command,omitempty"` // atmos terraform apply <component> -s <stack>.
	RiskLevel     string       `json:"risk_level,omitempty" yaml:"risk_level,omitempty"`         // low, medium, high.
	References    []string     `json:"references,omitempty" yaml:"references,omitempty"`         // AWS docs, CIS benchmarks, etc.
}

// CodeChange represents a specific code change in a Terraform file.
type CodeChange struct {
	FilePath string `json:"file_path" yaml:"file_path"`
	Line     int    `json:"line,omitempty" yaml:"line,omitempty"`
	Before   string `json:"before" yaml:"before"`
	After    string `json:"after" yaml:"after"`
}

// Report represents a complete security or compliance analysis report.
type Report struct {
	GeneratedAt    time.Time              `json:"generated_at" yaml:"generated_at"`
	Stack          string                 `json:"stack,omitempty" yaml:"stack,omitempty"`
	Component      string                 `json:"component,omitempty" yaml:"component,omitempty"`
	TotalFindings  int                    `json:"total_findings" yaml:"total_findings"`
	SeverityCounts map[Severity]int       `json:"severity_counts" yaml:"severity_counts"`
	Findings       []Finding              `json:"findings" yaml:"findings"`
	MappedCount    int                    `json:"mapped_count" yaml:"mapped_count"`
	UnmappedCount  int                    `json:"unmapped_count" yaml:"unmapped_count"`
	TagMapping     *AWSSecurityTagMapping `json:"-" yaml:"-"` // Display-only: configured tag keys for unmapped findings message.
	GroupFindings  bool                   `json:"-" yaml:"-"` // Display-only: group duplicate findings in Markdown output.
	Invocation     *ReportInvocation      `json:"-" yaml:"-"`
}

// ReportInvocation captures audit details for a CLI run.
type ReportInvocation struct {
	CommandLine         string
	Arguments           []string
	StartTimeUTC        time.Time
	EndTimeUTC          time.Time
	ExitCode            int
	ExitCodeDescription string
	WorkingDirectory    string
	ExecutionSuccessful bool
	AccountsScanned     []string
	RegionsScanned      []string
	StacksScanned       []string
}

// AWSSecurityTagMapping is re-exported from schema for use in reports.
type AWSSecurityTagMapping = schema.AWSSecurityTagMapping

// ComplianceReport represents a compliance posture report for a specific framework.
type ComplianceReport struct {
	GeneratedAt     time.Time           `json:"generated_at" yaml:"generated_at"`
	Stack           string              `json:"stack,omitempty" yaml:"stack,omitempty"`
	Framework       string              `json:"framework" yaml:"framework"`
	FrameworkTitle  string              `json:"framework_title" yaml:"framework_title"`
	TotalControls   int                 `json:"total_controls" yaml:"total_controls"`
	PassingControls int                 `json:"passing_controls" yaml:"passing_controls"`
	FailingControls int                 `json:"failing_controls" yaml:"failing_controls"`
	ScorePercent    float64             `json:"score_percent" yaml:"score_percent"`
	FailingDetails  []ComplianceControl `json:"failing_details" yaml:"failing_details"`
}

// ComplianceControl represents a single compliance control and its status.
type ComplianceControl struct {
	ControlID   string       `json:"control_id" yaml:"control_id"`
	Title       string       `json:"title" yaml:"title"`
	Severity    Severity     `json:"severity" yaml:"severity"`
	Component   string       `json:"component,omitempty" yaml:"component,omitempty"`
	Stack       string       `json:"stack,omitempty" yaml:"stack,omitempty"`
	Remediation *Remediation `json:"remediation,omitempty" yaml:"remediation,omitempty"`
}

// QueryOptions contains the filter options for fetching security findings.
type QueryOptions struct {
	Stack       string
	Component   string
	Severity    []Severity
	Source      Source
	Framework   string
	MaxFindings int
	Region      string
	NoAI        bool
}

// MaxFindingsForLookup is the default max findings when looking up a specific finding by ID.
const MaxFindingsForLookup = 500

// OutputFormat represents the desired output format.
type OutputFormat string

const (
	FormatMarkdown OutputFormat = "markdown"
	FormatJSON     OutputFormat = "json"
	FormatYAML     OutputFormat = "yaml"
	FormatCSV      OutputFormat = "csv"
	FormatSARIF    OutputFormat = "sarif"
	FormatOCSF     OutputFormat = "ocsf"
)

// ParseOutputFormat validates a format string and returns the corresponding OutputFormat.
func ParseOutputFormat(format string) (OutputFormat, error) {
	switch strings.ToLower(format) {
	case "markdown", "md", "":
		return FormatMarkdown, nil
	case "json":
		return FormatJSON, nil
	case "yaml", "yml":
		return FormatYAML, nil
	case "csv":
		return FormatCSV, nil
	case "sarif":
		return FormatSARIF, nil
	case "ocsf":
		return FormatOCSF, nil
	default:
		return "", errUtils.ErrAWSSecurityInvalidFormat
	}
}
