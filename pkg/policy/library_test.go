package policy

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSampleLibrary runs the actual distributed files, not copied expressions.
// Every sample rule must have cases; adding an untested rule fails this test.
func TestSampleLibrary(t *testing.T) {
	tests := map[string][]sampleCase{
		"terraform-safety/protect-stateful-resources": {
			{"update", samplePlan("aws_db_instance", []string{"update"}, map[string]any{}, nil), NotApplicable, nil},
			{"delete", samplePlan("aws_db_instance", []string{"delete"}, nil, nil), Deny, nil},
			{"replacement", samplePlan("aws_s3_bucket", []string{"create", "delete"}, map[string]any{}, nil), Deny, nil},
		},
		"terraform-safety/create-before-destroy": {
			{"safe replacement", samplePlan("aws_instance", []string{"create", "delete"}, map[string]any{}, nil), NotApplicable, nil},
			{"unsafe replacement", samplePlan("aws_instance", []string{"delete", "create"}, map[string]any{}, nil), Deny, nil},
		},
		"terraform-safety/limit-change-count": {
			{"at threshold", countPlan(10), NotApplicable, nil},
			{"over threshold", countPlan(11), Deny, nil},
			{"empty", countPlan(0), NotApplicable, nil},
		},
		"terraform-safety/disallow-resource-types": {
			{"allowed", samplePlan("aws_instance", []string{"create"}, map[string]any{}, nil), NotApplicable, nil},
			{"forbidden", samplePlan("aws_iam_access_key", []string{"create"}, map[string]any{}, nil), Deny, nil},
			{"remediation", samplePlan("aws_iam_access_key", []string{"delete"}, nil, nil), NotApplicable, nil},
		},
		"terraform-safety/allowed-instance-types": {
			{"approved", samplePlan("aws_instance", []string{"create"}, map[string]any{"instance_type": "t3.micro"}, nil), NotApplicable, nil},
			{"unapproved", samplePlan("aws_instance", []string{"update"}, map[string]any{"instance_type": "m5.24xlarge"}, nil), Deny, nil},
			{"unknown", samplePlan("aws_instance", []string{"create"}, map[string]any{}, map[string]any{"instance_type": true}), Deny, nil},
			{"unknown mask wins", samplePlan("aws_instance", []string{"create"}, map[string]any{"instance_type": "t3.micro"}, map[string]any{"instance_type": true}), Deny, nil},
			{"deleted", samplePlan("aws_instance", []string{"delete"}, nil, nil), NotApplicable, nil},
		},
		"terraform-safety/require-owner-tag": {
			{"owned", tagPlan(map[string]any{"Owner": "platform"}, nil), NotApplicable, nil},
			{"absent", tagPlan(map[string]any{}, nil), Deny, nil},
			{"empty", tagPlan(map[string]any{"Owner": ""}, nil), Deny, nil},
			{"null", tagPlan(nil, nil), Deny, nil},
			{"unknown map", tagPlan(map[string]any{"Owner": "platform"}, map[string]any{"tags": true}), Deny, nil},
			{"unknown owner", tagPlan(map[string]any{"Owner": "platform"}, map[string]any{"tags": map[string]any{"Owner": true}}), Deny, nil},
		},
		"github-safety/reject-fork-plan": {
			{"same repository", countPlan(0), NotApplicable, sampleCI("acme/infra", "acme/infra")},
			{"fork", countPlan(0), Deny, sampleCI("contributor/infra", "acme/infra")},
			{"missing evidence", countPlan(0), EvaluationError, nil},
		},
		"lifecycle/lint-successful-plan": operationCases("after.terraform.plan", "success", "dev", "failure"),
		"lifecycle/scan-production": {
			{"production", sampleOperation("before.terraform.plan", "success", "prod-us"), Allow, nil},
			{"development", sampleOperation("before.terraform.plan", "success", "dev-us"), NotApplicable, nil},
			{"other component", sampleOperation("before.helm.apply", "success", "prod-us"), NotApplicable, nil},
		},
		"lifecycle/callback-after-apply": operationCases("after.helm.apply", "success", "dev", "failure"),
		"lifecycle/cleanup-failed-apply": operationCases("after.kubernetes.apply", "failure", "dev", "success"),
		"lifecycle/deny-production-delete": {
			{"production", sampleOperation("before.helm.delete", "success", "prod-us"), Deny, nil},
			{"development", sampleOperation("before.helm.delete", "success", "dev-us"), NotApplicable, nil},
			{"unrelated command", sampleOperation("before.helm.apply", "success", "prod-us"), NotApplicable, nil},
		},
	}
	var engine Engine
	require.NoError(t, engine.Register(CELEngine, &CEL{}))
	seen := map[string]bool{}
	for _, family := range []string{"terraform", "github", "lifecycle"} {
		bundle, err := LoadDirectory(filepath.Join("..", "..", "examples", "policies", family))
		require.NoError(t, err)
		for _, rule := range bundle.Rules {
			cases, exists := tests[rule.ID]
			require.True(t, exists, "sample %s has no tests", rule.ID)
			seen[rule.ID] = true
			for _, tc := range cases {
				t.Run(rule.ID+"/"+tc.name, func(t *testing.T) {
					data, err := json.Marshal(tc.input)
					require.NoError(t, err)
					decision, err := engine.Evaluate(context.Background(), &Request{Kind: rule.On, Input: data, CI: tc.ci}, []Rule{rule})
					if tc.want == EvaluationError {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
					require.Equal(t, tc.want, decision.Status)
					if tc.want == Allow {
						require.NotEmpty(t, decision.Findings[0].Effects.Hooks)
						for _, hook := range decision.Findings[0].Effects.Hooks {
							require.Contains(t, bundle.Hooks, hook.Ref)
						}
					}
				})
			}
		}
	}
	require.Len(t, seen, len(tests), "test refers to a removed sample")
}

type sampleCase struct {
	name  string
	input map[string]any
	want  DecisionStatus
	ci    *CISnapshot
}

func samplePlan(resource string, actions []string, after, unknown map[string]any) map[string]any {
	if unknown == nil {
		unknown = map[string]any{}
	}
	return map[string]any{"format_version": "1.2", "planned_values": map[string]any{}, "resource_changes": []any{
		map[string]any{
			"address": resource + ".example", "mode": "managed", "type": resource, "name": "example",
			"change": map[string]any{"actions": actions, "after": after, "after_unknown": unknown},
		},
	}}
}

func countPlan(count int) map[string]any {
	plan := samplePlan("terraform_data", []string{"create"}, map[string]any{}, nil)
	resource := plan["resource_changes"].([]any)[0]
	resources := []any{}
	for range count {
		resources = append(resources, resource)
	}
	plan["resource_changes"] = resources
	return plan
}

func tagPlan(tags any, unknown map[string]any) map[string]any {
	return samplePlan("aws_instance", []string{"create"}, map[string]any{"tags": tags}, unknown)
}

func sampleCI(head, base string) *CISnapshot {
	data, _ := json.Marshal(map[string]any{"pull_request": map[string]any{
		"head": map[string]any{"repo": map[string]any{"full_name": head}},
		"base": map[string]any{"repo": map[string]any{"full_name": base}},
	}})
	return &CISnapshot{Provider: "github-actions", Schema: "github-actions/v1", Event: Dataset{Data: data, Status: Available, Complete: true}}
}

func sampleOperation(event, status, stack string) map[string]any {
	return map[string]any{"event": event, "status": status, "stack": stack}
}

func operationCases(event, status, stack, otherStatus string) []sampleCase {
	return []sampleCase{
		{"selected", sampleOperation(event, status, stack), Allow, nil},
		{"other outcome", sampleOperation(event, otherStatus, stack), NotApplicable, nil},
		{"other event", sampleOperation("before.terraform.init", status, stack), NotApplicable, nil},
	}
}
