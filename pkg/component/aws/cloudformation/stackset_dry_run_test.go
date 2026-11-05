package cloudformation

import "testing"

func TestRunOperation_DryRunStackSetsAndObservability(t *testing.T) {
	for _, operation := range []Operation{
		OperationStackSetCreate, OperationStackSetUpdate, OperationStackSetDelete,
		OperationStackSetInstances, OperationTree, OperationLogs, OperationWatch,
	} {
		t.Run(string(operation), func(t *testing.T) {
			assertOperationDryRun(t, operation, &stackSpec{StackName: "vpc"}, nil)
		})
	}
}
