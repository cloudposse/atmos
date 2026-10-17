package cloudformation

// Operation is an aws/cloudformation component lifecycle operation.
type Operation string

const (
	// OperationRender renders the local template client-side (no API calls).
	OperationRender Operation = "render"
	// OperationDiff creates (or reuses) a changeset and previews the changes an apply would make.
	OperationDiff Operation = "diff"
	// OperationApply executes the changeset, creating or updating the stack.
	OperationApply Operation = "apply"
	// OperationDelete deletes the stack.
	OperationDelete Operation = "delete"
	// OperationValidate calls the server-side ValidateTemplate API.
	OperationValidate Operation = "validate"
	// OperationOutput renders the deployed stack's Outputs.
	OperationOutput Operation = "output"
)
