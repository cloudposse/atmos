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
	// OperationChangesetCreate creates a changeset and leaves it for manual review/execution.
	OperationChangesetCreate Operation = "changeset-create"
	// OperationChangesetExecute executes a previously-created, named changeset.
	OperationChangesetExecute Operation = "changeset-execute"
	// OperationChangesetList lists a stack's changesets.
	OperationChangesetList Operation = "changeset-list"
	// OperationChangesetDelete deletes a named changeset.
	OperationChangesetDelete Operation = "changeset-delete"
	// OperationDriftDetect starts drift detection against the deployed stack.
	OperationDriftDetect Operation = "drift-detect"
	// OperationDriftDescribe renders the results of the most recent drift detection.
	OperationDriftDescribe Operation = "drift-describe"
	// OperationGetTemplate fetches the deployed stack's template.
	OperationGetTemplate Operation = "get-template"
	// OperationGetPolicy fetches the deployed stack's policy.
	OperationGetPolicy Operation = "get-policy"
)
