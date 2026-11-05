package exec

import (
	"bytes"
	"context"
	"io"

	"github.com/cloudposse/atmos/pkg/policy/recovery"
	"github.com/cloudposse/atmos/pkg/schema"
)

func executeShellRetryPolicy(info *schema.ConfigAndStacksInfo, phase string, invoke invokeShellCommandFunc, options ...ShellCommandOption) (bool, error) {
	componentType, eventCommand := shellErrorEvent(info, phase)
	runner, err := recovery.New(info, componentType, eventCommand)
	if err != nil || runner == nil {
		return err != nil, err
	}
	capture := errorPolicyCapture(options)
	metadataOut, metadataErr := execMetadataOutputCaptureFromOpts(options...)
	var detailedResult error
	err = runner.Run(capture.ctx, phase, info.ComponentRetrySection, func() recovery.Result {
		capture.stdout.Reset()
		capture.stderr.Reset()
		resetExecMetadataBufs(metadataOut, metadataErr)
		err := invoke(capture.options...)
		code := resolveExitCode(err)
		// A detailed plan result is not an error event. Preserve its exit status.
		if componentType == "terraform" && phase == "plan" && code == 2 {
			detailedResult = err
			return recovery.Result{ExitCode: code}
		}
		return recovery.Result{Err: err, ExitCode: code, Stdout: capture.stdout.String(), Stderr: capture.stderr.String()}
	})
	if err == nil && detailedResult != nil {
		return true, detailedResult
	}
	return true, err
}

type policyCapture struct {
	ctx            context.Context
	options        []ShellCommandOption
	stdout, stderr *bytes.Buffer
}

func errorPolicyCapture(options []ShellCommandOption) policyCapture {
	var config shellCommandConfig
	for _, option := range options {
		option(&config)
	}
	ctx := config.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	stdoutWriter, stderrWriter := io.Writer(&syncWriter{w: stdout}), io.Writer(&syncWriter{w: stderr})
	if config.stdoutCapture != nil {
		stdoutWriter = io.MultiWriter(config.stdoutCapture, stdoutWriter)
	}
	if config.stderrCapture != nil {
		stderrWriter = io.MultiWriter(config.stderrCapture, stderrWriter)
	}
	capture := append([]ShellCommandOption{}, options...)
	capture = append(capture, WithStdoutCapture(stdoutWriter), WithStderrCapture(stderrWriter))
	return policyCapture{ctx: ctx, options: capture, stdout: stdout, stderr: stderr}
}

func shellErrorEvent(info *schema.ConfigAndStacksInfo, phase string) (string, string) {
	componentType := info.ComponentType
	if componentType == "" {
		componentType = "terraform"
	}
	eventCommand := phase
	if phase == info.SubCommand && info.SubCommand2 != "" {
		eventCommand += "." + info.SubCommand2
	}
	return componentType, eventCommand
}
