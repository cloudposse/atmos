// Package planfile binds policy evaluation to saved Terraform artifacts. It does
// not collect CI facts, execute hooks, resolve approvals or coordinate other runs.
package planfile

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/process"
)

// Guard evaluates rules using the configured pure engine. Show uses an isolated
// process runner so raw JSON never passes through terminal masking or CI capture.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock_runner_test.go -package=planfile github.com/cloudposse/atmos/pkg/process Runner
type Guard struct {
	Engine *policy.Engine
	Rules  []policy.Rule
	Show   process.Runner
	CI     *policy.CISnapshot
}

// Invocation is the prepared Terraform process, not a command to re-enter Atmos.
// Args must contain an explicit -out for plan or a final saved-plan argument for apply.
type Invocation struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
}

// Execute runs one governed plan/apply. The callback preserves the existing
// command executor's streams/retries. A separate gate error must NEVER be remapped
// as a Terraform exit code or treated as best-effort reporting.
func (g *Guard) Execute(ctx context.Context, invocation *Invocation, run func([]string) process.Result) (process.Result, error) {
	defer perf.Track(nil, "planfile.Guard.Execute")()

	if err := g.validate(invocation, run); err != nil {
		return process.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return process.Result{}, err
	}
	selection, err := selectArtifact(invocation)
	if err != nil {
		return process.Result{}, err
	}
	directory, err := os.MkdirTemp(invocation.Dir, ".atmos-policy-plan-*")
	if err != nil {
		return process.Result{}, fmt.Errorf("%w: create private artifact directory: %w", errUtils.ErrPolicyPlan, err)
	}
	defer os.RemoveAll(directory)
	privatePath, err := filepath.Abs(filepath.Join(directory, "planfile"))
	if err != nil {
		return process.Result{}, fmt.Errorf("%w: resolve artifact: %w", errUtils.ErrPolicyPlan, err)
	}
	args := selection.replace(invocation.Args, privatePath)
	if invocation.Args[0] == "apply" {
		return g.apply(ctx, invocation, selection, privatePath, run)
	}
	result := run(args)
	if !successfulPlan(&result) {
		return result, fmt.Errorf("%w: planning did not succeed", errUtils.ErrPolicyPlan)
	}
	if err := g.evaluate(ctx, invocation, privatePath); err != nil {
		return result, err
	}
	if err := publish(privatePath, selection.path); err != nil {
		return result, fmt.Errorf("%w: publish approved plan: %w", errUtils.ErrPolicyPlan, err)
	}
	return result, nil
}

func (g *Guard) validate(invocation *Invocation, run func([]string) process.Result) error {
	if g.Engine == nil || g.Show == nil || len(g.Rules) == 0 || run == nil || invocation == nil {
		return fmt.Errorf("%w: unconfigured plan gate", errUtils.ErrPolicyInvalid)
	}
	return nil
}

func (g *Guard) apply(
	ctx context.Context, invocation *Invocation, selection artifactSelection, privatePath string, run func([]string) process.Result,
) (process.Result, error) {
	if err := copyArtifact(selection.path, privatePath); err != nil {
		return process.Result{}, fmt.Errorf("%w: snapshot saved plan: %w", errUtils.ErrPolicyPlan, err)
	}
	if err := g.evaluate(ctx, invocation, privatePath); err != nil {
		return process.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return process.Result{}, err
	}
	return run(selection.replace(invocation.Args, privatePath)), nil
}

func (g *Guard) evaluate(ctx context.Context, invocation *Invocation, path string) error {
	before, err := artifactDigest(path)
	if err != nil {
		return fmt.Errorf("%w: read saved artifact: %w", errUtils.ErrPolicyPlan, err)
	}
	data, err := readPlanJSON(ctx, g.Show, invocation, path)
	if err != nil {
		return err
	}
	decision, evalErr := g.Engine.Evaluate(ctx, &policy.Request{Kind: policy.TerraformPlan, Input: data, CI: g.CI}, g.Rules)
	if evalErr != nil {
		// CEL runtime errors can contain input values. Do not leak them through CLI/CI logs.
		return fmt.Errorf("%w: plan evaluation failed (input details withheld)", errUtils.ErrPolicyEvaluation)
	}
	if err := policy.EnforceDecision(decision); err != nil {
		return fmt.Errorf("%w: rules %s", err, blockingRules(decision))
	}
	after, err := artifactDigest(path)
	if err != nil || before != after {
		return fmt.Errorf("%w: saved artifact changed during evaluation", errUtils.ErrPolicyPlan)
	}
	return ctx.Err()
}

func blockingRules(decision policy.Decision) string {
	var ids []string
	for _, finding := range decision.Findings {
		if finding.Status != policy.NotApplicable {
			ids = append(ids, finding.RuleID)
		}
	}
	return strings.Join(ids, ", ")
}

func successfulPlan(result *process.Result) bool {
	return result.Started && !result.Canceled && !result.Signaled &&
		((result.ExitCode == 0 && result.Err == nil) || result.ExitCode == 2)
}

func artifactDigest(path string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	file, err := openArtifact(path)
	if err != nil {
		return digest, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return digest, err
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}
