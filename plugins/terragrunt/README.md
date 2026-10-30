# Native Terragrunt configuration plugin

This development package evaluates Terragrunt HCL with Terragrunt Go modules. Atmos lists the resulting units as Terraform components and executes them with its Terraform/OpenTofu lifecycle. The Terragrunt executable is not used.

This is an unmerged implementation with limited compatibility. It is not a published plugin release.

## Build and run

From the repository root:

```shell
atmos build
./build/atmos plugin build plugins/terragrunt
./build/atmos plugin test plugins/terragrunt
./build/atmos plugin test plugins/terragrunt --integration
```

On Windows, use `build/atmos.exe`. CI bootstraps the host with `go build -o build/atmos.exe .`, then uses the same plugin commands.

The default suite runs Go unit tests, Starlark affected-selection fixtures, and the installed-release scenario. It requires Go and Git, but no Python, shell, Terraform, or Terragrunt executable. The integration suite requires OpenTofu or Terraform on PATH and applies only built-in `terraform_data` resources with local state in temporary directories. Its scenarios cover:

- inventory, apply, unchanged plans, fresh dependency outputs, and reverse-order destroy (`native.star`);
- `before_hook`, `after_hook`, `error_hook`, and `extra_arguments` ordering, scoping, failure exit codes, and implicit init;
- saved-plan apply, including rejection of a replaced plan file, a changed working directory, and edited configuration;
- `!terraform.output` and `!terraform.state` reads of a Terragrunt unit, including the never-applied case, which must not create state;
- Git (`git::file://`) and local module sources, including re-provisioning after the source changes.

Neither suite calls the Terragrunt CLI. Use `plugin test --build` after changing plugin code, because the suite runs the built plugin binary and a stale binary can still pass.

Configure a project with:

```yaml
components:
  terraform:
    base_path: components/terraform
    command: tofu
    terragrunt:
      enabled: true
      mode: native
      discovery:
        enabled: true
        included_paths: [live/**]
plugins:
  packages:
    terragrunt:
      path: /absolute/path/to/this/plugins/terragrunt
```

Run `atmos list instances`, `atmos describe stacks`, and `atmos terraform plan --all`. No per-unit YAML is required. Discovery uses an in-memory overlay for explicit stacks. It does not create `.terragrunt-stack` or read dependency state. Apply generates explicit stacks and evaluates each unit after its dependencies. Discovered units use Atmos workdirs to isolate generated files by stack and component. State retains its Terragrunt backend identity.

Preparation receives Atmos's sanitized environment after authentication, the selected executable, toolchain PATH, and managed Terraform RC configuration. Child commands use the invocation PATH. Neither identity changes nor HCL evaluation replace the plugin process environment.

## Affected selection

The `describe affected --base <ref>` and `terraform plan --affected --ref <ref>` paths discover HEAD and BASE independently. Both checkouts use the invocation's trusted plugin and discovery scope. Historical HCL never reuses the current inventory or reads dependency state.

The affected acceptance runner uses temporary Git repositories and dry runs. It checks additions, deletions, renames, changed dependency targets, shared includes, local module files, nested explicit stacks, mixed YAML/HCL graphs, stack filters, and dependent depth. Both output and ordering edges participate in `--include-dependents`. The runner requires Git but no Terraform or Terragrunt executable.

Source tracking covers files read during partial discovery, declared HCL, local module directories, stack template provenance, and files and globs named by input expressions. A dynamic path that cannot be resolved without running the expression falls back to treating the unit as affected by any change in its scope. Effect-dependent discovery still fails. This is not a claim that every Terragrunt expression has complete affected provenance.

## Scope

The fixtures cover local sources, local backend paths, includes, locals, nested stack values, literal generation, order and output dependencies, command-scoped mocks, fresh outputs, and native bulk execution. Unit tests cover absolute and relative workdir overrides, ownership conflicts, repeated provisioning without state loss, concurrent environment isolation, unit-relative child commands, cancellation, and rejected effects. The plugin workflow runs its unit tests on Linux, macOS, and Windows. These checks do not establish a compatibility percentage.

The host executes `extra_arguments` and `before_hook`, `after_hook`, and `error_hook` blocks with the invocation's environment, identity, toolchain PATH, and cancellation. The plugin only evaluates them. Hooks run once per Atmos phase, and an implicit `init` matches hooks for `init`, not for the requested command. Hook output goes to stderr. Workspace selection, `providers lock`, dependency-output reads, and `atmos terraform shell` do not run hooks. A missing required var file fails before any process starts. Saved-plan apply reuses the execution data recorded at plan time and requires the original working directory.

Remote module sources (`git::https`, `git::http`, `git::ssh`, `git::file`, `https`, `http`) keep their getter URI for the Atmos source provisioner. Local module sources are re-copied on every run (`source.ttl: 0s`) so edits take effect. Registry sources and remote stack catalogs are not supported. Backend configuration is adopted, including cloud backends, but Atmos does not provision the backend resources.

Unsupported semantics fail before any infrastructure command and never switch executors. Currently rejected: `terraform_binary`; `errors`, `exclude`, feature flags, and IAM role or web-identity settings; `remote_state` with `disable_init` or encryption; inline modules without `terraform.source`; output dependencies with a non-default workspace during discovery; the `read-config`, `terragrunt-read-config`, and `init-from-module` hook lifecycles; hooks that run a built-in integration such as `tflint`; and an invalid `on_errors` expression. Executable mode, typed provider flags and help, exact cty types and sensitivity, scoped Terragrunt logging, migration and setup generation, and Atmos Pro integration remain in the PRDs.

Only enabled/mode/provider/discovery settings are accepted by this package. Additional plugin settings and typed CLI option handling remain planned. Discovery that needs command execution, decryption, network access, or identity-dependent effects fails. The native plugin is trusted code, not an OS sandbox.

## Maintained upstream facade

The package pins Terragrunt v1.2.1. Some required upstream facilities are in `internal` packages. The local `bridge/` module uses the import path `github.com/gruntwork-io/terragrunt/pkg/atmosbridge`, which satisfies Go's internal-package import rule. This is an Atmos-maintained facade, not a public upstream Go API or an upstream-published module. A checked-in relative module replacement resolves it to this package's source.

The plugin also uses this checkout's SDK through a relative module replacement. Normal Go module commands work without temporary source patches or module-cache changes. Go 1.27.3 is selected by the plugin module:

```shell
go -C plugins/terragrunt test ./...
go -C plugins/terragrunt/bridge test ./...
```

See [the Terragrunt PRD](../../docs/prd/terragrunt-support.md) and [the shared component contract](../../docs/prd/plugins/extensions/component-engines.md) for the release requirements.
