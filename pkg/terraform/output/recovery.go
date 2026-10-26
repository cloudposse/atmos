package output

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"

	"github.com/hashicorp/terraform-exec/tfexec"

	"github.com/cloudposse/atmos/pkg/policy/recovery"
	"github.com/cloudposse/atmos/pkg/schema"
)

// policyRunner gives each terraform-exec call its own event and retry budget.
type policyRunner struct {
	TerraformRunner
	policies map[string]*recovery.Runner
	stderr   *quietModeWriter
}

func withRetryPolicies(base TerraformRunner, info *schema.ConfigAndStacksInfo, stderr *quietModeWriter) (TerraformRunner, error) {
	runner := &policyRunner{TerraformRunner: base, policies: make(map[string]*recovery.Runner), stderr: stderr}
	for _, command := range []string{"init", "workspace.select", "workspace.new", "output"} {
		policy, err := recovery.New(info, "terraform", command)
		if err != nil {
			return nil, err
		}
		if policy != nil {
			runner.policies[command] = policy
		}
	}
	if len(runner.policies) == 0 {
		return base, nil
	}
	return runner, nil
}

//nolint:lintroller // Delegates instrumentation to recovery.Runner.Run.
func (r *policyRunner) Init(ctx context.Context, opts ...tfexec.InitOption) error {
	return r.run(ctx, "init", func() error { return r.TerraformRunner.Init(ctx, opts...) })
}

//nolint:lintroller // Delegates instrumentation to recovery.Runner.Run.
func (r *policyRunner) InitWithFlags(ctx context.Context, reconfigure, upgrade bool) error {
	return r.run(ctx, "init", func() error { return runTerraformInit(ctx, r.TerraformRunner, reconfigure, upgrade) })
}

//nolint:lintroller // Delegates instrumentation to recovery.Runner.Run.
func (r *policyRunner) WorkspaceSelect(ctx context.Context, workspace string, opts ...tfexec.WorkspaceSelectOption) error {
	return r.run(ctx, "workspace.select", func() error { return r.TerraformRunner.WorkspaceSelect(ctx, workspace, opts...) })
}

//nolint:lintroller // Delegates instrumentation to recovery.Runner.Run.
func (r *policyRunner) WorkspaceNew(ctx context.Context, workspace string, opts ...tfexec.WorkspaceNewCmdOption) error {
	return r.run(ctx, "workspace.new", func() error { return r.TerraformRunner.WorkspaceNew(ctx, workspace, opts...) })
}

//nolint:lintroller // Delegates instrumentation to recovery.Runner.Run.
func (r *policyRunner) Output(ctx context.Context, opts ...tfexec.OutputOption) (map[string]tfexec.OutputMeta, error) {
	var outputs map[string]tfexec.OutputMeta
	err := r.run(ctx, "output", func() error {
		var err error
		outputs, err = r.TerraformRunner.Output(ctx, opts...)
		return err
	})
	return outputs, err
}

func (r *policyRunner) run(ctx context.Context, command string, invoke func() error) error {
	runner := r.policies[command]
	if runner == nil {
		return invoke()
	}
	var stdout, stderr bytes.Buffer
	r.SetStdout(&stdout)
	var stderrWriter io.Writer = &stderr
	if r.stderr != nil {
		stderrWriter = io.MultiWriter(r.stderr, &stderr)
	}
	r.SetStderr(stderrWriter)
	defer r.restoreStreams()
	return runner.Run(ctx, command, nil, func() recovery.Result {
		stdout.Reset()
		stderr.Reset()
		r.stderr.Reset()
		err := invoke()
		var exitError *exec.ExitError
		code := 0
		if errors.As(err, &exitError) {
			code = exitError.ExitCode()
		}
		return recovery.Result{Err: err, Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}
	})
}

func (r *policyRunner) restoreStreams() {
	r.SetStdout(nil)
	var stderr io.Writer
	if r.stderr != nil {
		stderr = r.stderr
	}
	r.SetStderr(stderr)
}
