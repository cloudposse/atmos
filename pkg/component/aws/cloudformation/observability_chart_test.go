package cloudformation

import (
	"strings"
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
)

func TestRenderEventChart_SeparatesNestedStackResources(t *testing.T) {
	const firstStack = "arn:aws:cloudformation:us-east-1:123456789012:stack/child-a/first"
	const secondStack = "arn:aws:cloudformation:us-east-1:123456789012:stack/child-b/second"
	events := []cfntypes.StackEvent{
		{StackId: awsString(firstStack), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		{StackId: awsString(secondStack), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusDeleteInProgress},
		{StackId: awsString(secondStack), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusDeleteComplete},
		{StackId: awsString(firstStack), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
	}

	out := captureStdout(t, func() { renderEventChart(events) })

	// Rows are labeled with the stack name, not its ARN; the two stacks still get separate rows.
	assert.Equal(t, []string{
		"child-a/Bucket                 CREATE_IN_PROGRESS -> CREATE_COMPLETE",
		"child-b/Bucket                 DELETE_IN_PROGRESS -> DELETE_COMPLETE",
	}, strings.Split(strings.TrimRight(out, "\n"), "\n"))
	assert.NotContains(t, out, "arn:aws:cloudformation")
}

// When two different stacks share a name (a deleted and re-created stack), the name alone would
// merge their rows, so the ARN is kept for disambiguation; stacks with unique names keep short labels.
func TestRenderEventChart_KeepsARNOnlyForNameCollisions(t *testing.T) {
	const oldStack = "arn:aws:cloudformation:us-east-1:123456789012:stack/app/old-guid"
	const newStack = "arn:aws:cloudformation:us-east-1:123456789012:stack/app/new-guid"
	const uniqueStack = "arn:aws:cloudformation:us-east-1:123456789012:stack/other/guid"
	events := []cfntypes.StackEvent{
		{StackId: awsString(oldStack), StackName: awsString("app"), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusDeleteComplete},
		{StackId: awsString(newStack), StackName: awsString("app"), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
		{StackId: awsString(uniqueStack), StackName: awsString("other"), LogicalResourceId: awsString("Role"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
	}

	out := captureStdout(t, func() { renderEventChart(events) })

	assert.Contains(t, out, oldStack+"/Bucket")
	assert.Contains(t, out, newStack+"/Bucket")
	assert.Contains(t, out, "other/Role")
	assert.NotContains(t, out, uniqueStack)
}

func TestRenderEventChart_SingleStack(t *testing.T) {
	events := []cfntypes.StackEvent{
		{StackId: awsString("root"), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		{StackId: awsString("root"), LogicalResourceId: awsString("Role"), ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		{StackId: awsString("root"), LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
		{StackId: awsString("root"), LogicalResourceId: awsString("Role"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
	}

	out := captureStdout(t, func() { renderEventChart(events) })

	assert.Equal(t, "root/Bucket                    CREATE_IN_PROGRESS -> CREATE_COMPLETE\n"+
		"root/Role                      CREATE_IN_PROGRESS -> CREATE_COMPLETE\n", out)
}

func TestRenderEventChart_NoStackIdentity(t *testing.T) {
	events := []cfntypes.StackEvent{
		{LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusCreateInProgress},
		{LogicalResourceId: awsString("Bucket"), ResourceStatus: cfntypes.ResourceStatusCreateComplete},
	}

	out := captureStdout(t, func() { renderEventChart(events) })

	assert.Equal(t, "Bucket                         CREATE_IN_PROGRESS -> CREATE_COMPLETE\n", out)
}

func TestRenderEventChart_Empty(t *testing.T) {
	out := captureStdout(t, func() { renderEventChart(nil) })

	assert.Empty(t, out)
}
