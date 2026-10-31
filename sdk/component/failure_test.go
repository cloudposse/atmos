package component_test

import (
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cloudposse/atmos/sdk/component"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestFailureRoundTripsThroughStatus(t *testing.T) {
	t.Parallel()
	err := component.NewFailure("unsupported_setting", "Unsupported Terragrunt setting").
		Operation("prepare").Unit("live/dev", "vpc").Key("terraform_binary").At("live/dev/vpc/terragrunt.hcl", 7, 3).
		Hint("Remove terraform_binary.").Hint("Set components.terraform.command instead.").
		SafeDetail("terraform_binary is not supported.").Err(codes.Unimplemented)
	if status.Code(err) != codes.Unimplemented || status.Convert(err).Message() != "Unsupported Terragrunt setting" {
		t.Fatalf("unexpected status: %v", err)
	}
	detail, ok := component.FailureDetail(err)
	if !ok {
		t.Fatal("detail missing")
	}
	checks := map[string][2]any{
		"code": {detail.Code, "unsupported_setting"}, "unit": {detail.Unit, "vpc"}, "stack": {detail.Stack, "live/dev"},
		"key": {detail.ConfigKey, "terraform_binary"}, "operation": {detail.Operation, "prepare"}, "safe": {detail.UserSafe, true},
		"line": {detail.Location.Line, uint32(7)}, "column": {detail.Location.Column, uint32(3)}, "file": {detail.Location.File, "live/dev/vpc/terragrunt.hcl"},
		"hints": {detail.Hints, []string{"Remove terraform_binary.", "Set components.terraform.command instead."}},
	}
	for name, pair := range checks {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Errorf("%s: got %v want %v", name, pair[0], pair[1])
		}
	}
}

func TestFailureDefaultsToUnsafeError(t *testing.T) {
	t.Parallel()
	detail := component.NewFailure("c", "s").Detail("value was hunter2").Diagnostic().Detail
	if detail.UserSafe || detail.Severity != pluginv1.Severity_SEVERITY_ERROR {
		t.Fatalf("unexpected defaults: %v", detail)
	}
}

func TestFailureWarningDiagnostic(t *testing.T) {
	t.Parallel()
	diagnostic := component.NewFailure("skipped", "Directory skipped").Warning().Diagnostic()
	if diagnostic.Detail.Severity != pluginv1.Severity_SEVERITY_WARNING || diagnostic.Message != "Directory skipped" {
		t.Fatalf("unexpected diagnostic: %v", diagnostic)
	}
}

func TestFailureDetailMissing(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{"plain": fmt.Errorf("plain"), "nodetails": status.Error(codes.Internal, "x"), "nil": nil} {
		if _, ok := component.FailureDetail(err); ok {
			t.Errorf("%s: unexpected detail", name)
		}
	}
}

func TestFailureDetailThroughWrapping(t *testing.T) {
	t.Parallel()
	err := component.NewFailure("c", "s").Err(codes.InvalidArgument)
	if _, ok := component.FailureDetail(fmt.Errorf("wrapped: %w", err)); !ok {
		t.Fatal("detail lost through wrapping")
	}
}

func TestFailureProtoIsIndependent(t *testing.T) {
	t.Parallel()
	failure := component.NewFailure("c", "s").Hint("a").At("f", 1, 2)
	first := failure.Proto()
	first.Hints[0], first.Location.File = "mutated", "mutated"
	second := failure.Proto()
	if second.Hints[0] != "a" || second.Location.File != "f" {
		t.Fatalf("copy aliases the builder: %v", second)
	}
}
