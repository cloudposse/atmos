package kubernetes

type Operation string

const (
	OperationRender Operation = "render"
	OperationDiff   Operation = "diff"
	OperationApply  Operation = "apply"
	OperationDelete Operation = "delete"
)
