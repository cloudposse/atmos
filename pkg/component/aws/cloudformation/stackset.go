package cloudformation

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
)

// kindAwsStackSet is the provision-target kind for multi-account/multi-region
// StackSet delivery, matching the aws/s3 packaging-target vocabulary.
const kindAwsStackSet = "aws/stackset"

// errWrapFmt is the shared fmt.Errorf format for wrapping an SDK error with
// this file's stackset-failed sentinel.
const errWrapFmt = "%w: %w"

// defaultPermissionModel is used when a stackset target doesn't set permission_model.
const defaultPermissionModel = "SELF_MANAGED"

// stackSetOperationPollInterval/Timeout bound how long `stackset create/update/
// delete/instances` waits for the async StackSet operation they kick off. Vars
// (not consts) so tests can shrink them.
var (
	stackSetOperationPollInterval = 3 * time.Second
	stackSetOperationTimeout      = 30 * time.Minute
)

// stackSetConfig is the resolved `kind: aws/stackset` provision target.
type stackSetConfig struct {
	Name                  string
	Accounts              []string
	Regions               []string
	PermissionModel       string
	AdministrationRoleArn string
	ExecutionRoleName     string
}

// resolveStackSetTarget finds the single `kind: aws/stackset` provision target,
// or the one named by flagTarget when more than one is declared. Unlike
// apply's implicit direct-deploy default, there is no sensible default here —
// a StackSet target must always be declared explicitly.
func resolveStackSetTarget(provisionSection map[string]any, flagTarget string) (*stackSetConfig, error) {
	targets, _ := provisionSection["targets"].(map[string]any)
	stackSetTargets := make(map[string]map[string]any)
	for name, value := range targets {
		block, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := block["kind"].(string); kind == kindAwsStackSet {
			stackSetTargets[name] = block
		}
	}

	if flagTarget != "" {
		block, ok := stackSetTargets[flagTarget]
		if !ok {
			return nil, fmt.Errorf("%w: %q is not a `kind: aws/stackset` provision target", errUtils.ErrInvalidAwsCloudFormationSettings, flagTarget)
		}
		return stackSetConfigFromTarget(flagTarget, block), nil
	}

	switch len(stackSetTargets) {
	case 0:
		return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
			WithExplanation("No `kind: aws/stackset` provision target is declared.").
			WithHint("Add a `provision.targets.<name>: {kind: aws/stackset, accounts: [...], regions: [...]}` entry.").
			Err()
	case 1:
		for name, block := range stackSetTargets {
			return stackSetConfigFromTarget(name, block), nil
		}
	}
	return nil, errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
		WithExplanation("Multiple `kind: aws/stackset` provision targets are declared; selection is ambiguous.").
		WithHint("Pass --target <name> to select one.").
		Err()
}

// stackSetConfigFromTarget extracts a stackSetConfig from a resolved provision target block.
func stackSetConfigFromTarget(name string, block map[string]any) *stackSetConfig {
	cfg := &stackSetConfig{Name: name, PermissionModel: defaultPermissionModel}
	if v, ok := block["permission_model"].(string); ok && v != "" {
		cfg.PermissionModel = v
	}
	if v, ok := block["administration_role_arn"].(string); ok {
		cfg.AdministrationRoleArn = v
	}
	if v, ok := block["execution_role_name"].(string); ok {
		cfg.ExecutionRoleName = v
	}
	cfg.Accounts = toStringSlice(block["accounts"])
	cfg.Regions = toStringSlice(block["regions"])
	return cfg
}

// toStringSlice normalizes a []any (as decoded from YAML) to []string.
func toStringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// runStackSetCreate creates the StackSet, then creates stack instances in the
// configured accounts/regions when any are declared.
func runStackSetCreate(ctx context.Context, client CloudFormationClient, spec *stackSpec, ssCfg *stackSetConfig, summary map[string]any) (map[string]any, error) {
	_, err := client.CreateStackSet(ctx, &cloudformation.CreateStackSetInput{
		StackSetName:          awsString(spec.StackName),
		TemplateBody:          awsString(spec.TemplateBody),
		Parameters:            spec.Parameters,
		Capabilities:          spec.Capabilities,
		Tags:                  spec.Tags,
		PermissionModel:       cfntypes.PermissionModels(ssCfg.PermissionModel),
		AdministrationRoleARN: nilIfEmpty(ssCfg.AdministrationRoleArn),
		ExecutionRoleName:     nilIfEmpty(ssCfg.ExecutionRoleName),
	})
	if err != nil {
		return summary, fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationStackSetFailed, err)
	}
	_ = data.Writeln(fmt.Sprintf("%s: stackset created", spec.StackName))
	summary["stackset_name"] = spec.StackName

	if len(ssCfg.Accounts) == 0 || len(ssCfg.Regions) == 0 {
		return summary, nil
	}
	return runStackSetInstancesCreate(ctx, client, spec.StackName, ssCfg, summary)
}

// runStackSetInstancesCreate creates stack instances for a StackSet in the
// configured accounts/regions and waits for the operation to finish.
func runStackSetInstancesCreate(ctx context.Context, client CloudFormationClient, stackSetName string, ssCfg *stackSetConfig, summary map[string]any) (map[string]any, error) {
	out, err := client.CreateStackInstances(ctx, &cloudformation.CreateStackInstancesInput{
		StackSetName: awsString(stackSetName),
		Accounts:     ssCfg.Accounts,
		Regions:      ssCfg.Regions,
	})
	if err != nil {
		return summary, fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationStackSetFailed, err)
	}
	status, err := pollStackSetOperation(ctx, client, stackSetName, stringValue(out.OperationId))
	summary["operation_status"] = string(status)
	if err != nil {
		return summary, err
	}
	_ = data.Writeln(fmt.Sprintf("%s: %d instance(s) across %d region(s): %s", stackSetName, len(ssCfg.Accounts)*len(ssCfg.Regions), len(ssCfg.Regions), status))
	return summary, nil
}

// runStackSetUpdate updates the StackSet's template/parameters/capabilities and
// waits for the update operation to propagate to every existing stack instance.
func runStackSetUpdate(ctx context.Context, client CloudFormationClient, spec *stackSpec, ssCfg *stackSetConfig, summary map[string]any) (map[string]any, error) {
	out, err := client.UpdateStackSet(ctx, &cloudformation.UpdateStackSetInput{
		StackSetName:          awsString(spec.StackName),
		TemplateBody:          awsString(spec.TemplateBody),
		Parameters:            spec.Parameters,
		Capabilities:          spec.Capabilities,
		Tags:                  spec.Tags,
		AdministrationRoleARN: nilIfEmpty(ssCfg.AdministrationRoleArn),
		ExecutionRoleName:     nilIfEmpty(ssCfg.ExecutionRoleName),
	})
	if err != nil {
		return summary, fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationStackSetFailed, err)
	}
	status, err := pollStackSetOperation(ctx, client, spec.StackName, stringValue(out.OperationId))
	summary["operation_status"] = string(status)
	if err != nil {
		return summary, err
	}
	_ = data.Writeln(fmt.Sprintf("%s: stackset updated (%s)", spec.StackName, status))
	return summary, nil
}

// runStackSetDelete deletes every stack instance (required before a StackSet
// itself can be deleted), then deletes the StackSet.
func runStackSetDelete(ctx context.Context, client CloudFormationClient, stackSetName string, summary map[string]any) (map[string]any, error) {
	instances, err := listStackSetInstances(ctx, client, stackSetName)
	if err != nil {
		return summary, err
	}
	if len(instances) > 0 {
		accounts, regions := instanceAccountsRegions(instances)
		out, err := client.DeleteStackInstances(ctx, &cloudformation.DeleteStackInstancesInput{
			StackSetName: awsString(stackSetName),
			Accounts:     accounts,
			Regions:      regions,
			RetainStacks: awsBool(false),
		})
		if err != nil {
			return summary, fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationStackSetFailed, err)
		}
		if _, err := pollStackSetOperation(ctx, client, stackSetName, stringValue(out.OperationId)); err != nil {
			return summary, err
		}
	}

	if _, err := client.DeleteStackSet(ctx, &cloudformation.DeleteStackSetInput{StackSetName: awsString(stackSetName)}); err != nil {
		return summary, fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationStackSetFailed, err)
	}
	_ = data.Writeln(fmt.Sprintf("%s: stackset deleted", stackSetName))
	summary["stackset_name"] = stackSetName
	return summary, nil
}

// runStackSetInstances lists a StackSet's stack instances.
func runStackSetInstances(ctx context.Context, client CloudFormationClient, stackSetName string, summary map[string]any) (map[string]any, error) {
	instances, err := listStackSetInstances(ctx, client, stackSetName)
	if err != nil {
		return summary, err
	}
	summary["instances"] = instances
	if len(instances) == 0 {
		_ = data.Writeln(fmt.Sprintf("%s: no stack instances", stackSetName))
		return summary, nil
	}
	for i := range instances {
		inst := &instances[i]
		_ = data.Writeln(fmt.Sprintf("  %-14s %-14s %-10s %s", stringValue(inst.Account), stringValue(inst.Region), inst.Status, stringValue(inst.StackId)))
	}
	return summary, nil
}

// listStackSetInstances fetches every stack instance for a StackSet, paginating through NextToken.
func listStackSetInstances(ctx context.Context, client CloudFormationClient, stackSetName string) ([]cfntypes.StackInstanceSummary, error) {
	defer perf.Track(nil, "cloudformation.listStackSetInstances")()

	var instances []cfntypes.StackInstanceSummary
	var nextToken *string
	for {
		out, err := client.ListStackInstances(ctx, &cloudformation.ListStackInstancesInput{
			StackSetName: awsString(stackSetName),
			NextToken:    nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationStackSetFailed, err)
		}
		instances = append(instances, out.Summaries...)
		if out.NextToken == nil {
			return instances, nil
		}
		nextToken = out.NextToken
	}
}

// instanceAccountsRegions extracts the unique account/region sets covered by a
// list of stack instances, for a full DeleteStackInstances call.
func instanceAccountsRegions(instances []cfntypes.StackInstanceSummary) ([]string, []string) {
	accountSet := map[string]bool{}
	regionSet := map[string]bool{}
	for i := range instances {
		accountSet[stringValue(instances[i].Account)] = true
		regionSet[stringValue(instances[i].Region)] = true
	}
	return mapKeys(accountSet), mapKeys(regionSet)
}

// mapKeys returns the keys of a map[string]bool as a slice.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// pollStackSetOperation polls DescribeStackSetOperation until the operation
// reaches a terminal status (SUCCEEDED/FAILED/STOPPED).
func pollStackSetOperation(ctx context.Context, client CloudFormationClient, stackSetName, operationID string) (cfntypes.StackSetOperationStatus, error) {
	deadline := time.Now().Add(stackSetOperationTimeout)
	for {
		out, err := client.DescribeStackSetOperation(ctx, &cloudformation.DescribeStackSetOperationInput{
			StackSetName: awsString(stackSetName),
			OperationId:  awsString(operationID),
		})
		if err != nil {
			return "", fmt.Errorf(errWrapFmt, errUtils.ErrAwsCloudFormationStackSetFailed, err)
		}

		status := out.StackSetOperation.Status
		switch status {
		case cfntypes.StackSetOperationStatusSucceeded:
			return status, nil
		case cfntypes.StackSetOperationStatusFailed, cfntypes.StackSetOperationStatusStopped:
			return status, fmt.Errorf("%w: stackset operation %s ended in status %s", errUtils.ErrAwsCloudFormationStackSetFailed, operationID, status)
		}

		if time.Now().After(deadline) {
			return status, fmt.Errorf("%w: timed out waiting for stackset operation %s", errUtils.ErrAwsCloudFormationStackSetFailed, operationID)
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(stackSetOperationPollInterval):
		}
	}
}

// nilIfEmpty returns nil for an empty string, or a pointer to s otherwise —
// several StackSet SDK fields distinguish "not set" from "set to empty".
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
