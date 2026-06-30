package helm

// Operation is a Helm component lifecycle operation.
type Operation string

const (
	// OperationTemplate renders chart templates to manifests (no cluster contact).
	OperationTemplate Operation = "template"
	// OperationDiff previews the changes an apply would make.
	OperationDiff Operation = "diff"
	// OperationApply installs or upgrades the release (or delivers to a target).
	OperationApply Operation = "apply"
	// OperationDelete uninstalls the release.
	OperationDelete Operation = "delete"
)

const (
	// The dirPerm const is the permission mode used when creating directories.
	dirPerm = 0o755
)

// repositorySource describes where a repository entry came from.
type repositorySource string

const (
	repositorySourceGlobal    repositorySource = "global"
	repositorySourceComponent repositorySource = "component"
	repositorySourceDirect    repositorySource = "direct"
)

// chartRepository is the normalized form of a declarative Helm chart repository.
type chartRepository struct {
	Name                  string
	URL                   string
	Username              string
	Password              string // #nosec G117 -- Helm repository credentials are user-provided configuration passed through to Helm.
	PassCredentialsAll    bool
	CertFile              string
	KeyFile               string
	CAFile                string
	InsecureSkipTLSVerify bool
	Source                repositorySource
}
