package exec

import (
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// resolvesTerraformStateFunctions reports whether this pass will still evaluate at least one
// of `!terraform.state` / `!terraform.output`, i.e. whether it can still perform a credentialed
// backend read. It is false only when **both** are skipped, which is what a caller that opted
// out of credentialed reads entirely looks like (`--skip terraform.state --skip terraform.output`).
//
// Skipping just one of the pair — e.g. `atmos describe stacks --skip terraform.state` — leaves
// the other resolving against a remote backend, so the hints below still apply and this must
// return true. Treating a partial skip as credential-free would drop the guidance from exactly
// the case where a user is already mid-way through applying it.
func (p *describeStacksProcessor) resolvesTerraformStateFunctions() bool {
	for _, functionName := range []string{u.AtmosYamlFuncTerraformState, u.AtmosYamlFuncTerraformOutput} {
		if !slices.Contains(p.skip, strings.TrimPrefix(functionName, "!")) {
			return true
		}
	}
	return false
}

// explainRepositoryWideYAMLFunctionFailure enriches a YAML-function failure with the flags that
// resolve it, but only when the describe is scanning the whole repository (no `--stack` filter)
// with credentialed backend reads enabled.
//
// It fires only under `--error-mode=strict`, which the nil onWarning identifies: warn/silent
// mode degrades an unreadable backend to `(computed)` (see isRecoverableInWarnMode), so the
// first hint below — "drop --error-mode=strict" — would be nonsense advice for a caller
// already in warn mode that failed on something non-recoverable.
//
// An unfiltered `atmos describe stacks` evaluates every component in every stack. In a
// multi-account repository each stage keeps its Terraform state in its own account, so a single
// set of credentials can never read all of them: one `AccessDenied` aborts the entire command
// with a raw S3/backend error that says nothing about how to proceed. See
// https://github.com/cloudposse/atmos/issues/2566 and
// docs/fixes/2026-07-28-inventory-commands-read-remote-tfstate.md.
//
// Two cases get the error back untouched, because the `describe stacks` remedies below would be
// wrong advice for them:
//   - The caller scoped the run with `--stack`: they asked for that stack specifically, so the
//     underlying error is the whole answer.
//   - The terraform state/output functions are already skipped: the caller is an inventory `list`
//     command, which never performs these reads and does not accept these flags the same way.
//
// Enrichment is transparent to `errors.Is`: callers branch on sentinels propagated from the inner
// describe (e.g. ErrCircularDependency from a `!terraform.state` cycle), and those must keep
// matching through the added hints.
func (p *describeStacksProcessor) explainRepositoryWideYAMLFunctionFailure(err error, componentName, stackName string) error {
	if err == nil || p.filterByStack != "" || !p.resolvesTerraformStateFunctions() || p.onWarning != nil {
		return err
	}

	return errUtils.Build(err).
		WithHintf("Atmos evaluated the YAML functions of every component in every stack; this one failed for component `%s` in stack `%s`.", componentName, stackName).
		WithHint("Drop `--error-mode=strict` to degrade values Atmos cannot read to `(computed)` and keep going — the default `warn` mode does this and reports a summary.").
		WithHintf("Or narrow the run to fewer components: `atmos describe stacks --stack %s`. Note this only limits which components are evaluated — a `!terraform.state` inside that stack can still name another stack or account.", stackName).
		WithHint("Or skip the credential-backed functions: `atmos describe stacks --skip terraform.state --skip terraform.output`.").
		WithHint("Or disable YAML function processing entirely: `atmos describe stacks --process-functions=false`.").
		WithContext("component", componentName).
		WithContext("stack", stackName).
		Err()
}
