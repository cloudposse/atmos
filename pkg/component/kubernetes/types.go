package kubernetes

// Operation identifies a Kubernetes component runtime operation.
type Operation string

const (
	// OperationRender renders manifests without contacting the cluster.
	OperationRender Operation = "render"
	// OperationDiff computes a server-side dry-run diff against the cluster.
	OperationDiff Operation = "diff"
	// OperationApply applies manifests to the cluster.
	OperationApply Operation = "apply"
	// OperationDelete deletes the component's objects from the cluster.
	OperationDelete Operation = "delete"
	// OperationValidate validates rendered manifests (offline, plus an optional server-side dry-run).
	OperationValidate Operation = "validate"
)
