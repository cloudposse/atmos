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
	// OperationFmt formats the local template in place (or checks formatting with --check).
	OperationFmt Operation = "fmt"
	// OperationStackSetCreate creates a StackSet and, when accounts/regions are
	// configured on the selected target, its initial stack instances.
	OperationStackSetCreate Operation = "stackset-create"
	// OperationStackSetUpdate updates a StackSet's template/parameters/capabilities.
	OperationStackSetUpdate Operation = "stackset-update"
	// OperationStackSetDelete deletes every stack instance, then the StackSet itself.
	OperationStackSetDelete Operation = "stackset-delete"
	// OperationStackSetInstances lists a StackSet's stack instances.
	OperationStackSetInstances Operation = "stackset-instances"
	// OperationTree renders the nested-stack dependency tree for a component.
	OperationTree Operation = "tree"
	// OperationLogs renders the combined event log across a stack and its nested stacks.
	OperationLogs Operation = "logs"
	// OperationWatch attaches to a stack's in-progress (or already-terminal) operation and streams events.
	OperationWatch Operation = "watch"
)
