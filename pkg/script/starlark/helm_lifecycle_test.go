package starlark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// These tests execute the scenario's checked-in Starlark with process mocks, so
// validation errors can be exercised without Helm, Docker or Kubernetes.
type lifecycleReply struct {
	command string
	stdout  string
	stderr  string
	code    int
}

func runLifecycleCheck(t *testing.T, source, runnerOS string, replies ...lifecycleReply) error {
	t.Helper()
	dir, err := filepath.Abs("../../../tests/fixtures/scenarios/helm-lifecycle")
	require.NoError(t, err)
	runner := NewMockRunner(gomock.NewController(t))
	calls := make([]any, 0, len(replies))
	for _, reply := range replies {
		calls = append(calls, runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
			assert.Equal(t, "atmos-under-test", spec.Command)
			assert.Equal(t, reply.command, strings.Join(spec.Args, " "))
			assert.Contains(t, spec.Env, "NO_COLOR=1")
			_, _ = io.WriteString(spec.Streams.Stdout, reply.stdout)
			_, _ = io.WriteString(spec.Streams.Stderr, reply.stderr)
			result := process.Result{Started: true, ExitCode: reply.code}
			if reply.code != 0 {
				result.Err = errUtils.ErrProcessWaitFailed
			}
			return result
		}))
	}
	gomock.InOrder(calls...)
	var stdout, stderr bytes.Buffer
	_, err = New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
		WorkingDirectory: dir, Source: source, Stdout: &stdout, Stderr: &stderr,
		Env: map[string]string{"ATMOS_CLI_PATH": "atmos-under-test", "RUNNER_OS": runnerOS},
	})
	assert.Empty(t, stdout.String(), "child output must remain captured")
	assert.Empty(t, stderr.String(), "child diagnostics must remain captured")
	return err
}

func TestHelmLifecycleAbsence(t *testing.T) {
	t.Parallel()
	const source = `load("scripts/lifecycle.star", "assert_absent")
assert_absent("deployment", "demo", "demo")`
	for _, tc := range []struct {
		name, stdout, stderr string
		code                 int
		wantErr              bool
	}{
		{"missing", "", "", 0, false},
		{"present", `{"kind":"Deployment"}`, "", 0, true},
		{"forbidden", "", "Forbidden", 1, true},
		{"connection refused", "", "connection refused", 1, true},
		{"invalid resource", "", "the server doesn't have a resource type", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := runLifecycleCheck(t, source, "", lifecycleReply{
				command: "emulator exec kubernetes -s dev -- kubectl -n demo get deployment demo --ignore-not-found -o json",
				stdout:  tc.stdout, stderr: tc.stderr, code: tc.code,
			})
			assert.Equal(t, tc.wantErr, err != nil, "%v", err)
			if tc.stderr != "" {
				require.ErrorContains(t, err, tc.stderr)
			}
		})
	}
}

func TestHelmLifecycleJobCompletion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		json    string
		wantErr bool
	}{
		{`{"status":{"conditions":[{"type":"Complete","status":"True"}]}}`, false},
		{`{"status":{"conditions":[{"type":"Complete","status":"False"}]}}`, true},
		{`{"status":{"conditions":[{"type":"Failed","status":"True"}]}}`, true},
		{`{}`, true},
	} {
		err := runLifecycleCheck(t, `load("scripts/lifecycle.star", "assert_job_complete")
assert_job_complete()`, "", lifecycleReply{
			command: "emulator exec kubernetes -s dev -- kubectl -n demo-jobs get job demo-jobs-job -o json", stdout: tc.json,
		})
		assert.Equal(t, tc.wantErr, err != nil, "%v", err)
	}
}

func TestHelmLifecycleExpectedFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, text string
		code       int
		wantErr    bool
	}{
		{"expected", "failed to perform helm release operation: job demo-install-fail-failing-hook failed: BackoffLimitExceeded", 1, false},
		{"alternative diagnostic", "failed to perform helm release operation: job failed job=demo-install-fail-failing-hook reason=BackoffLimitExceeded", 1, false},
		{"unexpected success", "", 0, true},
		{"unrelated failure", "unable to authenticate", 1, true},
		{"wrong hook", "failed to perform helm release operation: job other failed: BackoffLimitExceeded", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := runLifecycleCheck(t, `load("scripts/lifecycle.star", "assert_failed_install")
assert_failed_install()`, "", lifecycleReply{
				command: "helm apply demo-install-fail -s dev --identity local-k3s", stderr: tc.text, code: tc.code,
			})
			assert.Equal(t, tc.wantErr, err != nil, "%v", err)
		})
	}
}

func TestHelmLifecycleRollback(t *testing.T) {
	t.Parallel()
	const source = `load("scripts/lifecycle.star", "assert_failed_upgrade")
assert_failed_upgrade()`
	const get = "emulator exec kubernetes -s dev -- kubectl -n demo get deployment demo -o json"
	state := func(replicas int) string {
		return fmt.Sprintf(`{"spec":{"replicas":%d,"template":{"spec":{"containers":[{"image":"nginx:1.27","ports":[{"containerPort":80}]}]}}}}`, replicas)
	}
	for _, tc := range []struct {
		name, runnerOS, diagnostic string
		after                      int
		wantErr, readAfter         bool
	}{
		{"restored", "Linux", "job demo-failing-hook failed: BackoffLimitExceeded", 1, false, true},
		{"changed state", "Linux", "job demo-failing-hook failed: BackoffLimitExceeded", 2, true, true},
		{"macOS timeout", "macOS", "context deadline exceeded", 1, false, true},
		{"macOS bad rollback", "macOS", "context deadline exceeded", 2, true, true},
		{"Linux timeout rejected", "Linux", "context deadline exceeded", 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			replies := []lifecycleReply{
				{command: get, stdout: state(1)},
				{command: "helm apply demo-upgrade-fail -s dev --identity local-k3s", stderr: "failed to perform helm release operation: " + tc.diagnostic, code: 1},
			}
			if tc.readAfter {
				replies = append(replies, lifecycleReply{command: get, stdout: state(tc.after)})
			}
			err := runLifecycleCheck(t, source, tc.runnerOS, replies...)
			assert.Equal(t, tc.wantErr, err != nil, "%v", err)
		})
	}
}

func TestHelmLifecycleStarlarkSteps(t *testing.T) {
	t.Parallel()
	path, err := filepath.Abs("../../../tests/fixtures/scenarios/helm-lifecycle/atmos.yaml")
	require.NoError(t, err)
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	var config struct {
		Commands []struct {
			Steps []yaml.Node `yaml:"steps"`
		} `yaml:"commands"`
	}
	require.NoError(t, yaml.Unmarshal(contents, &config))
	require.NotEmpty(t, config.Commands)
	count := 0
	for _, node := range config.Commands[0].Steps {
		if node.Kind == yaml.ScalarNode {
			continue
		}
		var step schema.WorkflowStep
		require.NoError(t, node.Decode(&step))
		if step.Interpreter != "starlark" {
			continue
		}
		count++
		assert.Equal(t, "script", step.Type)
		assert.Contains(t, step.Env, "ATMOS_CLI_PATH")
		assert.Contains(t, step.Env, "RUNNER_OS")
		assert.Contains(t, step.Script, `load("scripts/lifecycle.star"`)
	}
	require.Equal(t, 23, count)
}

func TestHelmLifecycleAdditionalChecks(t *testing.T) {
	t.Parallel()
	const prefix = "emulator exec kubernetes -s dev -- kubectl "
	for _, tc := range []struct {
		name, function, call string
		replies              []lifecycleReply
		wantErr              bool
	}{
		{
			name: "no dry-run release", function: "assert_release_records", call: "assert_release_records(False)",
			replies: []lifecycleReply{{command: prefix + "get secrets --all-namespaces -l owner=helm,name=demo -o json", stdout: `{"items":[]}`}},
		},
		{
			name: "dry-run persisted release", function: "assert_release_records", call: "assert_release_records(False)", wantErr: true,
			replies: []lifecycleReply{{command: prefix + "get secrets --all-namespaces -l owner=helm,name=demo -o json", stdout: `{"items":[{}]}`}},
		},
		{
			name: "delete dry-run retained release", function: "assert_release_records", call: `assert_release_records(True, "demo")`,
			replies: []lifecycleReply{{command: prefix + "get secrets -n demo -l owner=helm,name=demo -o json", stdout: `{"items":[{}]}`}},
		},
		{
			name: "CRD group", function: "assert_crd_group", call: "assert_crd_group()",
			replies: []lifecycleReply{{command: prefix + "get crd widgets.lifecycle.atmos.test -o json", stdout: `{"spec":{"group":"lifecycle.atmos.test"}}`}},
		},
		{
			name: "hook-only gate", function: "assert_hook_only", call: "assert_hook_only()",
			replies: []lifecycleReply{{command: prefix + "-n demo-hook-only get deployment demo-hook-only -o json", stdout: `{"status":{"conditions":[{"type":"Available","status":"False"}]}}`}},
		},
		{
			name: "hook-only unexpectedly ready", function: "assert_hook_only", call: "assert_hook_only()", wantErr: true,
			replies: []lifecycleReply{{command: prefix + "-n demo-hook-only get deployment demo-hook-only -o json", stdout: `{"status":{"conditions":[{"type":"Available","status":"True"}]}}`}},
		},
		{
			name: "release timeout", function: "assert_timeout", call: "assert_timeout()",
			replies: []lifecycleReply{{command: "helm apply demo-timeout -s dev --identity local-k3s", code: 1, stderr: "failed to perform helm release operation: context deadline exceeded"}},
		},
		{
			name: "timeout wrong reason", function: "assert_timeout", call: "assert_timeout()", wantErr: true,
			replies: []lifecycleReply{{command: "helm apply demo-timeout -s dev --identity local-k3s", code: 1, stderr: "failed to perform helm release operation: invalid chart"}},
		},
		{
			name: "ready dependency", function: "assert_dependency_ready", call: "assert_dependency_ready()",
			replies: []lifecycleReply{{command: prefix + "-n lifecycle-dag get configmap dag-dependent-dependency-observed -o json", stdout: `{"data":{"ready":"true"}}`}},
		},
		{
			name: "release progress", function: "assert_release_progress", call: `assert_release_progress("apply", "oss-ingress", "Applied Helm release")`,
			replies: []lifecycleReply{{command: "helm apply oss-ingress -s dev --identity local-k3s", stderr: "Applied Helm release oss-ingress in helm-oss"}},
		},
		{
			name: "missing progress", function: "assert_release_progress", call: `assert_release_progress("delete", "oss-ingress", "Deleted Helm release")`, wantErr: true,
			replies: []lifecycleReply{{command: "helm delete oss-ingress -s dev --identity local-k3s", stdout: "done"}},
		},
		{
			name: "admission hooks", function: "assert_admission_hooks", call: "assert_admission_hooks()",
			replies: []lifecycleReply{
				{command: prefix + "get validatingwebhookconfiguration oss-ingress-admission -o json", stdout: `{"webhooks":[{"clientConfig":{"caBundle":"certificate"}}]}`},
				{command: prefix + "-n helm-oss get jobs -l app.kubernetes.io/instance=oss-ingress -o json", stdout: `{"items":[]}`},
			},
		},
		{
			name: "admission hook job remains", function: "assert_admission_hooks", call: "assert_admission_hooks()", wantErr: true,
			replies: []lifecycleReply{
				{command: prefix + "get validatingwebhookconfiguration oss-ingress-admission -o json", stdout: `{"webhooks":[{"clientConfig":{"caBundle":"certificate"}}]}`},
				{command: prefix + "-n helm-oss get jobs -l app.kubernetes.io/instance=oss-ingress -o json", stdout: `{"items":[{}]}`},
			},
		},
		{
			name: "ingress upgrade", function: "assert_ingress_upgrade", call: "assert_ingress_upgrade()",
			replies: []lifecycleReply{{command: prefix + "-n helm-oss get deployment oss-ingress-controller -o json", stdout: `{"spec":{"replicas":2,"template":{"spec":{"containers":[{"env":[{"name":"ATMOS_UPGRADE_MARKER","value":"upgraded"}]}]}}}}`}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := fmt.Sprintf("load(\"scripts/lifecycle.star\", %q)\n%s", tc.function, tc.call)
			err := runLifecycleCheck(t, source, "", tc.replies...)
			assert.Equal(t, tc.wantErr, err != nil, "%v", err)
		})
	}
}

func TestHelmLifecycleSecretValuesStayCaptured(t *testing.T) {
	t.Parallel()
	for _, token := range []string{"atmos-oss-token-ABCD1234", "wrong"} {
		response := fmt.Sprintf(`{"spec":{"replicas":1,"template":{"spec":{"containers":[{"env":[
{"name":"ATMOS_SINGLE_LINE_SECRET","value":%q},
{"name":"ATMOS_MULTILINE_SECRET","value":"-----BEGIN PRIVATE KEY-----\nATMOS-OSS-LINE-A\nATMOS-OSS-LINE-B\n-----END PRIVATE KEY-----"}
]}]}}}}`, token)
		err := runLifecycleCheck(t, `load("scripts/lifecycle.star", "assert_deployed_secrets")
assert_deployed_secrets()`, "", lifecycleReply{
			command: "emulator exec kubernetes -s dev -- kubectl -n helm-oss get deployment oss-ingress-controller -o json", stdout: response,
		})
		if token == "wrong" {
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.NotContains(t, err.Error(), "PRIVATE KEY")
		} else {
			require.NoError(t, err)
		}
	}
}
