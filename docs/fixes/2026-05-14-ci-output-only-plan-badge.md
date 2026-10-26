# Fix: CI summary shows no-change badge for output-only plans

**Date:** 2026-05-14

## Problem

When an OpenTofu or Terraform plan reported only output value changes, the CI
job summary could show a `No Changes` heading and gray `NO_CHANGE` badge even
though the details correctly said:

```text
Output values will change. No infrastructure changes.
```

This happened on the stdout fallback path when the plan output contained
`Changes to Outputs:` without a resource change summary.

## Root Cause

The stdout parser set the overall result to `HasChanges=true` and populated the
changed-result text, but it did not preserve an explicit output-change signal in
the Terraform-specific parsed data.

The summary template context derived `HasOutputChanges` from parsed output
values only. JSON plan parsing has structured output changes, but stdout parsing
does not populate output values, so the template fell through to the no-change
heading and badge.

## Fix

Terraform CI parsing now records output changes explicitly in
`TerraformOutputData.HasOutputChanges`.

- JSON plan parsing sets the flag when `output_changes` are present.
- Stdout plan parsing sets the flag when `Changes to Outputs:` is present.
- Summary rendering uses the explicit flag, so output-only plans render the
  output-change heading and badge.

## Test Coverage

Added a regression test that renders the real `after.terraform.plan` CI summary
path from anonymized OpenTofu stdout containing `Changes to Outputs:`. The test
asserts that the summary uses `Output Changes Found` and `PLAN-OUTPUT_CHANGE`,
and does not use the no-change heading or badge.
