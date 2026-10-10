# Batch Terraform generators use resolved configuration context

## Problem

When `vars.stage` was computed by `!starlark`, the batch Terraform varfile and
backend generators retained the context and logical stack name captured before
configuration evaluation. For a stage resolving to `dev`, `--stacks=dev` silently
produced no file, while a file template containing `{stage}` attempted to use the
Starlark source as a filename. A `name_template` suffix could also turn derived
stack or workspace metadata into an invalid Starlark program.

## Fix

Both generators refresh their context and logical stack name from the resolved
component before stack filtering and output path expansion. The refresh preserves
the sanitized component name and component path, and supports both `name_pattern`
and `name_template`. Varfile generation also refreshes its workspace metadata.
Before evaluation, a derived stack or workspace value that is still Starlark uses
the physical stack name provisionally, preventing that metadata from executing as
another program.

This refresh applies to batch output filtering and naming after component
evaluation. Stack discovery and identities used to select a component before
evaluation remain outside the `!starlark` contract.

## Regression coverage

`TestTerraformGenerators_ResolvedContext` invokes both generator entry points with
temporary manifests. It checks logical and physical stack filters, unmatched
filters, unfiltered output, pattern and template stack names, and filenames using
`{stage}`, `{component}`, and `{component-path}`. Generated JSON must contain the
resolved values, including a backend bucket computed from the stage.
Invalid computed naming inputs must return an error without creating output.

Before the fix, logical filters skipped the expected output, filename expansion
failed with source text in the path, and a name-template suffix caused Starlark
evaluation to fail. The focused regression and existing batch generator
entry-point tests pass after the fix.
