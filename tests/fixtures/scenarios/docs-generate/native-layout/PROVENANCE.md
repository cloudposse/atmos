# README layout regression modules

These small modules isolate shapes found during the PR #3404 field test:

- `context-only`: Null Label has optional context inputs but no non-context inputs.
- `resource-only`: Kinesis Stream has managed resources without data sources.
- `data-only`: data sources must not create an empty managed-resources section.
- `required-only` and `optional-only`: variable groups must be guarded independently.
- `empty`: an existing module directory can contain no documented objects.
- `module-sources`: local child/parent paths, Git HTTPS sources, and an unversioned registry source.

They use the shared template in `../native-terraform/README.md.gomplate` through
`pkg/docs/native_layout_test.go`. They are standalone declarations, not copies of
upstream module implementations. No Terraform initialization or credentials are needed.

The representative fixture also checks rendered HTML for multiline types and
defaults, alongside the recovered-design golden comparison.

Run the command-level fixtures from this directory:

```shell
atmos docs generate context-only
atmos docs generate data-only
```
