package scanners

import "github.com/cloudposse/atmos/pkg/schema"

const (
	OnFailureWarn   = "warn"
	OnFailureFail   = "fail"
	OnFailureIgnore = "ignore"
)

const (
	FormatMarkdown = "markdown"
)

type Context struct {
	Name          string
	Command       string
	Args          []string
	Env           map[string]string
	BaseEnv       []string
	Format        string
	OnFailure     string
	CaptureStdout bool
	ResultHandler ResultHandler

	AtmosConfig   *schema.AtmosConfiguration
	Info          *schema.ConfigAndStacksInfo
	ToolchainPATH string

	OutputFile   string
	OutputDir    string
	ExitCode     int
	CommandError error
}

type Output struct {
	Artifact *Artifact
	Summary  *Summary
}

type Artifact struct {
	Name     string
	Body     []byte
	Format   string
	Metadata map[string]string
}

type SummaryStatus string

const (
	StatusSuccess SummaryStatus = "success"
	StatusWarning SummaryStatus = "warning"
	StatusFailure SummaryStatus = "failure"
)

type Summary struct {
	Kind     string
	Status   SummaryStatus
	Title    string
	Counts   map[string]int
	Body     string
	Findings []Finding
	SARIF    []byte
}

type Finding struct {
	Path     string
	Line     int
	Severity string
	RuleID   string
	Message  string
}

type ResultHandler func(ctx *Context) (*Summary, error)
