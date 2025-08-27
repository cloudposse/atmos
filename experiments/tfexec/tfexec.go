//go:build !linting
// +build !linting

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	tfexec "github.com/hashicorp/terraform-exec/tfexec"
)

// TerraformWrapper wraps tfexec.Terraform and injects TF_VAR_* dynamically.
type TerraformWrapper struct {
	*tfexec.Terraform
	customEnv []string // Stores the overridden environment variables
}

// SetEnv bypasses Terraform-Exec's environment filtering by storing the custom environment.
func (t *TerraformWrapper) SetEnv(env map[string]string) error {
	// Directly store env without any Terraform-Exec restrictions
	t.customEnv = make([]string, 0, len(env))
	for key, value := range env {
		t.customEnv = append(t.customEnv, fmt.Sprintf("%s=%s", key, value))
	}

	return nil
}

// RunTfCmd executes Terraform with the custom environment, bypassing Terraform-Exec’s restrictions.
func (t *TerraformWrapper) RunTfCmd(ctx context.Context, args ...string) error {
	env := t.customEnv
	if len(env) == 0 {
		env = os.Environ()
	}

	cmd := exec.CommandContext(ctx, t.Terraform.ExecPath(), args...)
	cmd.Dir = t.Terraform.WorkingDir()
	cmd.Env = env

	// ✅ Set the correct process attributes dynamically (cross-platform)
	cmd.SysProcAttr = newSysProcAttr()

	// Handle stdout/stderr to avoid deadlocks
	var errBuf strings.Builder

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	// Start command execution
	if err := cmd.Start(); err != nil {
		return err
	}

	// Read logs asynchronously to avoid blocking
	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		writeOutput(ctx, stdoutPipe, os.Stdout)
	}()
	go func() {
		defer wg.Done()
		writeOutput(ctx, stderrPipe, &errBuf)
	}()

	// Wait for output to complete before checking process exit
	wg.Wait()

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, errBuf.String())
	}

	return nil
}

// Licensed MPL-2.0 by HashiCorp.
func writeOutput(ctx context.Context, r io.ReadCloser, w io.Writer) error {
	// ReadBytes will block until bytes are read, which can cause a delay in
	// returning even if the command's context has been canceled. Use a separate
	// goroutine to prompt ReadBytes to return on cancel
	closeCtx, closeCancel := context.WithCancel(ctx)
	defer closeCancel()

	go func() {
		select {
		case <-ctx.Done():
			r.Close()
		case <-closeCtx.Done():
			return
		}
	}()

	buf := bufio.NewReader(r)

	for {
		line, err := buf.ReadBytes('\n')
		if len(line) > 0 {
			if _, err := w.Write(line); err != nil {
				return err
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}

			return err
		}
	}
}

// Init wraps Terraform Init and ensures the correct environment is set.
func (t *TerraformWrapper) Init(ctx context.Context, opts ...tfexec.InitOption) error {
	return t.RunTfCmd(ctx, "init")
}

// Apply wraps Terraform Apply and ensures the correct environment is set.
func (t *TerraformWrapper) Apply(ctx context.Context) error {
	return t.RunTfCmd(ctx, "apply", "-auto-approve")
}

// GetOutputs retrieves Terraform outputs and decodes JSON properly.
func (t *TerraformWrapper) GetOutputs(ctx context.Context) (map[string]any, error) {
	outputs, err := t.Terraform.Output(ctx)
	if err != nil {
		return nil, err
	}

	decodedOutputs := make(map[string]any)

	for key, output := range outputs {
		var decodedValue any
		if err := json.Unmarshal(output.Value, &decodedValue); err != nil {
			return nil, fmt.Errorf("failed to decode output %s: %v", key, err)
		}

		decodedOutputs[key] = decodedValue
	}

	return decodedOutputs, nil
}

func main() {
	// Get the current working directory
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting working directory:", err)
		return
	}

	// Dynamically find Terraform binary
	terraformPath, err := exec.LookPath("terraform") // Use "tofu" if using OpenTofu
	if err != nil {
		fmt.Println("Terraform binary not found:", err)
		return
	}

	// ✅ Correct argument order (wd first, then terraformPath)
	tf, err := tfexec.NewTerraform(wd, terraformPath)
	if err != nil {
		fmt.Println("Error initializing Terraform:", err)
		return
	}

	// Wrap Terraform with a custom environment injection
	wrappedTF := &TerraformWrapper{Terraform: tf}

	// ✅ Set environment variables, including TF_VAR_foo, bypassing Terraform-Exec’s block
	wrappedTF.SetEnv(map[string]string{
		"TF_VAR_foo": "bar",   // Overrides "foo" in Terraform
		"TF_LOG":     "DEBUG", // Enable Terraform debug logging
	})

	// Run Terraform Init
	err = wrappedTF.Init(context.Background())
	if err != nil {
		fmt.Println("Terraform Init failed:", err)
		return
	}

	// Run Terraform Apply
	err = wrappedTF.Apply(context.Background())
	if err != nil {
		fmt.Println("Terraform Apply failed:", err)
		return
	}

	// Retrieve and print outputs
	outputs, err := wrappedTF.GetOutputs(context.Background())
	if err != nil {
		fmt.Println("Error retrieving Terraform outputs:", err)
		return
	}

	// Print outputs properly formatted
	fmt.Println("Terraform Outputs:")

	for key, value := range outputs {
		fmt.Printf("%s: %v\n", key, value)
	}
}
