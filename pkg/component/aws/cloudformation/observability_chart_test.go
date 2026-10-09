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

	assert.Equal(t, []string{
		firstStack + "/Bucket CREATE_IN_PROGRESS -> CREATE_COMPLETE",
		secondStack + "/Bucket DELETE_IN_PROGRESS -> DELETE_COMPLETE",
	}, strings.Split(strings.TrimSpace(out), "\n"))
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
