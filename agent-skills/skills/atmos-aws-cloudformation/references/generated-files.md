# Generated CloudFormation Files

Set `components."aws/cloudformation".auto_generate_files: true` in `atmos.yaml`
(default false). A component `generate` map uses the shared engine: filenames map
to Go-template strings or maps serialized by extension (`.yaml`, `.json`, etc.).
Use long-form CloudFormation intrinsic functions in maps, such as `Ref: Marker`.

```yaml
components:
  "aws/cloudformation":
    generated:
      metadata:
        component: demo
      stack_name: "generated-{{ .vars.stage }}"
      path: generated/template.yaml
      generate:
        generated/template.yaml:
          AWSTemplateFormatVersion: "2010-09-09"
          Resources:
            Marker:
              Type: AWS::SSM::Parameter
              Properties:
                Type: String
                Value: "hello from {{ .vars.stage }}"
```

The source directory (`demo` here) must exist or be supplied by `source`.
Generation happens after source provisioning and subpath resolution, before
loading `path` and `stack_policy.file`. A generated template requires `path`;
an inline `template` can generate auxiliary files, including a JSON stack policy.
`template` and `path` remain mutually exclusive.

Enabled generation with a nonempty block automatically isolates local and
source-backed components into a workdir per stack/component instance. Distinct
instances sharing a component directory or source cannot overwrite each other's
generated files. Explicit `provision.workdir.enabled: false` is rejected.

Generation runs for `render`, `fmt`, `validate`, `diff`/`plan`, `apply`/`deploy`,
`changeset create`, `stackset create`/`update`, and `changeset execute` when a
configured stack policy needs loading. It does not run for `--dry-run` or
operations that only read/delete deployed state (`output`, `get`, `tree`, `logs`,
`watch`, `drift`, `delete`, other changeset verbs, or stackset delete/instances).
There is no standalone CloudFormation `generate` command.

The repository's `examples/cloudformation` includes `demo-generated`:

```shell
atmos --chdir=examples/cloudformation aws cloudformation render demo-generated -s local
```

This renders locally with no emulator or AWS credentials. Its generated template
and policy remain in the isolated workdir, leaving the shared `demo` directory
unchanged.
