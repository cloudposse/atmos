package exec

import (
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// resolvesTerraformStateFunctions reports whether this pass will actually evaluate
// `!terraform.state` / `!terraform.output`, i.e. whether the caller opted into credentialed
// backend reads. The inventory `list` commands always skip both (see
// skipCredentialBackedYAMLFunctionsForInventory in cmd/list/utils.go), so this is what
// distinguishes a `describe stacks` run from a `list` run sharing the same processor.
func (p *describeStacksProcessor) resolvesTerraformStateFunctions() bool {
	for _, functionName := range []string{u.AtmosYamlFuncTerraformState, u.AtmosYamlFuncTerraformOutput} {
		if slices.Contains(p.skip, strings.TrimPrefix(functionName, "!")) {
			return false
		}
	}
	return true
}

// explainRepositoryWideYAMLFunctionFailure enriches a YAML-function failure with the flags that
// resolve it, but only when the describe is scanning the whole repository (no `--stack` filter)
// with credentialed backend reads enabled.
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
	if err == nil || p.filterByStack != "" || !p.resolvesTerraformStateFunctions() {
		return err
	}

	return errUtils.Build(err).
		WithHintf("Atmos evaluated the YAML functions of every component in every stack; this one failed for component `%s` in stack `%s`.", componentName, stackName).
		WithHintf("Scope the command to a stack you hold credentials for: `atmos describe stacks --stack %s`.", stackName).
		WithHint("Or skip the credential-backed functions: `atmos describe stacks --skip terraform.state --skip terraform.output`.").
		WithHint("Or disable YAML function processing entirely: `atmos describe stacks --process-functions=false`.").
		WithContext("component", componentName).
		WithContext("stack", stackName).
		Err()
}
