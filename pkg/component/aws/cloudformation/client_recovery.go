package cloudformation

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

	"github.com/cloudposse/atmos/pkg/policy/recovery"
)

// recoveryClient decorates individual API calls so polling retries never replay mutations.
type recoveryClient struct {
	CloudFormationClient
	runner *recovery.Runner
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) CreateChangeSet(ctx context.Context, params *cloudformation.CreateChangeSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error) {
	return recovery.Call(ctx, c.runner, "CreateChangeSet", func() (*cloudformation.CreateChangeSetOutput, error) {
		return c.CloudFormationClient.CreateChangeSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DescribeChangeSet(ctx context.Context, params *cloudformation.DescribeChangeSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error) {
	return recovery.Call(ctx, c.runner, "DescribeChangeSet", func() (*cloudformation.DescribeChangeSetOutput, error) {
		return c.CloudFormationClient.DescribeChangeSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DeleteChangeSet(ctx context.Context, params *cloudformation.DeleteChangeSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.DeleteChangeSetOutput, error) {
	return recovery.Call(ctx, c.runner, "DeleteChangeSet", func() (*cloudformation.DeleteChangeSetOutput, error) {
		return c.CloudFormationClient.DeleteChangeSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) ExecuteChangeSet(ctx context.Context, params *cloudformation.ExecuteChangeSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.ExecuteChangeSetOutput, error) {
	return recovery.Call(ctx, c.runner, "ExecuteChangeSet", func() (*cloudformation.ExecuteChangeSetOutput, error) {
		return c.CloudFormationClient.ExecuteChangeSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) ListChangeSets(ctx context.Context, params *cloudformation.ListChangeSetsInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListChangeSetsOutput, error) {
	return recovery.Call(ctx, c.runner, "ListChangeSets", func() (*cloudformation.ListChangeSetsOutput, error) {
		return c.CloudFormationClient.ListChangeSets(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DescribeStacks(ctx context.Context, params *cloudformation.DescribeStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error) {
	return recovery.Call(ctx, c.runner, "DescribeStacks", func() (*cloudformation.DescribeStacksOutput, error) {
		return c.CloudFormationClient.DescribeStacks(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DescribeStackEvents(ctx context.Context, params *cloudformation.DescribeStackEventsInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStackEventsOutput, error) {
	return recovery.Call(ctx, c.runner, "DescribeStackEvents", func() (*cloudformation.DescribeStackEventsOutput, error) {
		return c.CloudFormationClient.DescribeStackEvents(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DeleteStack(ctx context.Context, params *cloudformation.DeleteStackInput, opts ...func(*cloudformation.Options)) (*cloudformation.DeleteStackOutput, error) {
	return recovery.Call(ctx, c.runner, "DeleteStack", func() (*cloudformation.DeleteStackOutput, error) {
		return c.CloudFormationClient.DeleteStack(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) ValidateTemplate(ctx context.Context, params *cloudformation.ValidateTemplateInput, opts ...func(*cloudformation.Options)) (*cloudformation.ValidateTemplateOutput, error) {
	return recovery.Call(ctx, c.runner, "ValidateTemplate", func() (*cloudformation.ValidateTemplateOutput, error) {
		return c.CloudFormationClient.ValidateTemplate(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) UpdateTerminationProtection(ctx context.Context, params *cloudformation.UpdateTerminationProtectionInput, opts ...func(*cloudformation.Options)) (*cloudformation.UpdateTerminationProtectionOutput, error) {
	return recovery.Call(ctx, c.runner, "UpdateTerminationProtection", func() (*cloudformation.UpdateTerminationProtectionOutput, error) {
		return c.CloudFormationClient.UpdateTerminationProtection(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) SetStackPolicy(ctx context.Context, params *cloudformation.SetStackPolicyInput, opts ...func(*cloudformation.Options)) (*cloudformation.SetStackPolicyOutput, error) {
	return recovery.Call(ctx, c.runner, "SetStackPolicy", func() (*cloudformation.SetStackPolicyOutput, error) {
		return c.CloudFormationClient.SetStackPolicy(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) GetTemplate(ctx context.Context, params *cloudformation.GetTemplateInput, opts ...func(*cloudformation.Options)) (*cloudformation.GetTemplateOutput, error) {
	return recovery.Call(ctx, c.runner, "GetTemplate", func() (*cloudformation.GetTemplateOutput, error) {
		return c.CloudFormationClient.GetTemplate(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) GetStackPolicy(ctx context.Context, params *cloudformation.GetStackPolicyInput, opts ...func(*cloudformation.Options)) (*cloudformation.GetStackPolicyOutput, error) {
	return recovery.Call(ctx, c.runner, "GetStackPolicy", func() (*cloudformation.GetStackPolicyOutput, error) {
		return c.CloudFormationClient.GetStackPolicy(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) ListStacks(ctx context.Context, params *cloudformation.ListStacksInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListStacksOutput, error) {
	return recovery.Call(ctx, c.runner, "ListStacks", func() (*cloudformation.ListStacksOutput, error) {
		return c.CloudFormationClient.ListStacks(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DetectStackDrift(ctx context.Context, params *cloudformation.DetectStackDriftInput, opts ...func(*cloudformation.Options)) (*cloudformation.DetectStackDriftOutput, error) {
	return recovery.Call(ctx, c.runner, "DetectStackDrift", func() (*cloudformation.DetectStackDriftOutput, error) {
		return c.CloudFormationClient.DetectStackDrift(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DescribeStackDriftDetectionStatus(ctx context.Context, params *cloudformation.DescribeStackDriftDetectionStatusInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStackDriftDetectionStatusOutput, error) {
	return recovery.Call(ctx, c.runner, "DescribeStackDriftDetectionStatus", func() (*cloudformation.DescribeStackDriftDetectionStatusOutput, error) {
		return c.CloudFormationClient.DescribeStackDriftDetectionStatus(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DescribeStackResourceDrifts(ctx context.Context, params *cloudformation.DescribeStackResourceDriftsInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStackResourceDriftsOutput, error) {
	return recovery.Call(ctx, c.runner, "DescribeStackResourceDrifts", func() (*cloudformation.DescribeStackResourceDriftsOutput, error) {
		return c.CloudFormationClient.DescribeStackResourceDrifts(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DescribeStackSet(ctx context.Context, params *cloudformation.DescribeStackSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStackSetOutput, error) {
	return recovery.Call(ctx, c.runner, "DescribeStackSet", func() (*cloudformation.DescribeStackSetOutput, error) {
		return c.CloudFormationClient.DescribeStackSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) CreateStackSet(ctx context.Context, params *cloudformation.CreateStackSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.CreateStackSetOutput, error) {
	return recovery.Call(ctx, c.runner, "CreateStackSet", func() (*cloudformation.CreateStackSetOutput, error) {
		return c.CloudFormationClient.CreateStackSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) UpdateStackSet(ctx context.Context, params *cloudformation.UpdateStackSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.UpdateStackSetOutput, error) {
	return recovery.Call(ctx, c.runner, "UpdateStackSet", func() (*cloudformation.UpdateStackSetOutput, error) {
		return c.CloudFormationClient.UpdateStackSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DeleteStackSet(ctx context.Context, params *cloudformation.DeleteStackSetInput, opts ...func(*cloudformation.Options)) (*cloudformation.DeleteStackSetOutput, error) {
	return recovery.Call(ctx, c.runner, "DeleteStackSet", func() (*cloudformation.DeleteStackSetOutput, error) {
		return c.CloudFormationClient.DeleteStackSet(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) CreateStackInstances(ctx context.Context, params *cloudformation.CreateStackInstancesInput, opts ...func(*cloudformation.Options)) (*cloudformation.CreateStackInstancesOutput, error) {
	return recovery.Call(ctx, c.runner, "CreateStackInstances", func() (*cloudformation.CreateStackInstancesOutput, error) {
		return c.CloudFormationClient.CreateStackInstances(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DeleteStackInstances(ctx context.Context, params *cloudformation.DeleteStackInstancesInput, opts ...func(*cloudformation.Options)) (*cloudformation.DeleteStackInstancesOutput, error) {
	return recovery.Call(ctx, c.runner, "DeleteStackInstances", func() (*cloudformation.DeleteStackInstancesOutput, error) {
		return c.CloudFormationClient.DeleteStackInstances(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) ListStackInstances(ctx context.Context, params *cloudformation.ListStackInstancesInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListStackInstancesOutput, error) {
	return recovery.Call(ctx, c.runner, "ListStackInstances", func() (*cloudformation.ListStackInstancesOutput, error) {
		return c.CloudFormationClient.ListStackInstances(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) DescribeStackSetOperation(ctx context.Context, params *cloudformation.DescribeStackSetOperationInput, opts ...func(*cloudformation.Options)) (*cloudformation.DescribeStackSetOperationOutput, error) {
	return recovery.Call(ctx, c.runner, "DescribeStackSetOperation", func() (*cloudformation.DescribeStackSetOperationOutput, error) {
		return c.CloudFormationClient.DescribeStackSetOperation(ctx, params, opts...)
	})
}

//nolint:lintroller // Delegates instrumentation to recovery.Call.
func (c *recoveryClient) ListStackResources(ctx context.Context, params *cloudformation.ListStackResourcesInput, opts ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error) {
	return recovery.Call(ctx, c.runner, "ListStackResources", func() (*cloudformation.ListStackResourcesOutput, error) {
		return c.CloudFormationClient.ListStackResources(ctx, params, opts...)
	})
}
