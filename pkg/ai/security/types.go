package security

import "time"

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
	ID                 string            `json:"id" yaml:"id"`
	Title              string            `json:"title" yaml:"title"`
	Description        string            `json:"description" yaml:"description"`
	Severity           Severity          `json:"severity" yaml:"severity"`
	Source             Source            `json:"source" yaml:"source"`
	ComplianceStandard string            `json:"compliance_standard,omitempty" yaml:"compliance_standard,omitempty"`
	ResourceARN        string            `json:"resource_arn" yaml:"resource_arn"`
	ResourceType       string            `json:"resource_type" yaml:"resource_type"`
	AccountID          string            `json:"account_id" yaml:"account_id"`
	Region             string            `json:"region" yaml:"region"`
	CreatedAt          time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at" yaml:"updated_at"`
	Mapping            *ComponentMapping `json:"mapping,omitempty" yaml:"mapping,omitempty"`
	Remediation        *Remediation      `json:"remediation,omitempty" yaml:"remediation,omitempty"`
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
type Remediation struct {
	Description   string         `json:"description" yaml:"description"`
	RootCause     string         `json:"root_cause,omitempty" yaml:"root_cause,omitempty"`
	StackChanges  map[string]any `json:"stack_changes,omitempty" yaml:"stack_changes,omitempty"`
	CodeChanges   []CodeChange   `json:"code_changes,omitempty" yaml:"code_changes,omitempty"`
	DeployCommand string         `json:"deploy_command,omitempty" yaml:"deploy_command,omitempty"`
	RiskLevel     string         `json:"risk_level,omitempty" yaml:"risk_level,omitempty"`
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
	GeneratedAt    time.Time        `json:"generated_at" yaml:"generated_at"`
	Stack          string           `json:"stack,omitempty" yaml:"stack,omitempty"`
	Component      string           `json:"component,omitempty" yaml:"component,omitempty"`
	TotalFindings  int              `json:"total_findings" yaml:"total_findings"`
	SeverityCounts map[Severity]int `json:"severity_counts" yaml:"severity_counts"`
	Findings       []Finding        `json:"findings" yaml:"findings"`
	MappedCount    int              `json:"mapped_count" yaml:"mapped_count"`
	UnmappedCount  int              `json:"unmapped_count" yaml:"unmapped_count"`
}

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
)
