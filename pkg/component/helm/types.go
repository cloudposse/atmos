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
