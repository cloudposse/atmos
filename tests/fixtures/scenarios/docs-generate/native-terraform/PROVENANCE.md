# README formatting regression fixture

`original.terraform-docs.yaml` was recovered from
[terraform-aws-components PR #1074](https://github.com/cloudposse/terraform-aws-components/pull/1074)
at commit `b299008468584f1e750ae61bf44a8680fae10907`:
[original file](https://github.com/cloudposse/terraform-aws-components/blob/b299008468584f1e750ae61bf44a8680fae10907/.terraform-docs.yaml).
Only trailing blank lines at the end of the YAML file are normalized for repository hooks;
the original template content and settings are unchanged.

`README.md.gomplate` adapts that template to Atmos's native `terraform` function.
It changes the module binding and uses Gomplate's helper names and argument order.
The README title and description come from `README.yaml`. Resource visibility is
chosen by the template rather than terraform-docs formatter settings.

`expected.md` is the reviewed output for the representative module in this directory,
with the absolute module path replaced by `.` and trailing spaces removed.
The regression test also renders the original content with terraform-docs and
compares nonblank lines, ignoring incidental surrounding whitespace and the
added README introduction. The reference renderer applies one explicit correction:
prefixing every multiline type line with the blockquote marker. The recovered
source file remains unchanged. This independently checks the adaptation's fidelity.

The adapted fixture additionally omits empty display groups, resolves local and
HTTPS module links, and uses `latest` for unversioned registry modules. Reduced
fixtures in `../native-layout/` exercise these cases. A Markdown-to-HTML regression
test checks that complex types and defaults remain separate code blocks.

Regenerate the reviewed output through the production renderer:

```shell
ATMOS_UPDATE_DOCS_GOLDEN=true go test ./pkg/docs -run '^TestGenerateDocument_NativeTerraformOriginalDesign$' -count=1
```

Run this command from the repository root, then review the resulting diff.

To try it, run `atmos docs generate readme` from this directory.
No Terraform initialization, provider installation, or cloud credentials are needed.
