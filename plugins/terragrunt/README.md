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

## Names, imports, patterns, and symlinks

User-facing rules are in [the migration guide](https://atmos.tools/migration/terragrunt#component-and-stack-names). The implementation points to know:

- Discovery derives the component from the unit directory name and the stack from the directory above it (`default` and `root` at the base path; the stack file's directory and the in-stack unit path for `terragrunt.stack.hcl`). `engine/discovery.go` owns these names.
- A `terragrunt://` import is resolved by the host while stack manifests are read (`pkg/plugin/adapter/configuration/imports.go`). The plugin returns the units; the host drops the derived stack and merges them as components of the importing manifest. Imports ignore `included_paths` and `excluded_paths`, and a unit that is imported is removed from automatic discovery.
- `included_paths` and `excluded_paths` share one matcher (`engine/discovery_paths.go`): a pattern selects what it matches and everything below it. Unmatched include patterns, shared `terragrunt.hcl` files without `terraform.source`, and skipped directory symlinks are returned as warning diagnostics, not failures.
- Symlinks follow Terragrunt v1.2.1: directory links are not followed (a warning names them) and a symlinked `terragrunt.hcl` is a unit at the link's location (`engine.logicalPath`).

## Affected selection

The `describe affected --base <ref>` and `terraform plan --affected --ref <ref>` paths discover HEAD and BASE independently. Both checkouts use the invocation's trusted plugin and discovery scope. Historical HCL never reuses the current inventory or reads dependency state.

The affected acceptance runner uses temporary Git repositories and dry runs. It checks additions, deletions, renames, changed dependency targets, shared includes, local module files, nested explicit stacks, mixed YAML/HCL graphs, stack filters, and dependent depth. Both output and ordering edges participate in `--include-dependents`. The runner requires Git but no Terraform or Terragrunt executable.

A unit is selected when any file it reads changes: `terragrunt.hcl` and its includes; the module directory (for a `dir//subdir` source the whole directory before `//`, and the target of a symlinked module); `extra_arguments` var files; files read by `generate` and `remote_state` blocks; hook scripts and working directories inside the project; `.terraform.lock.hcl` beside the unit; and files read by `file()`, `templatefile()`, `read_terragrunt_config()` and similar functions in `inputs`. Paths built from `get_terragrunt_dir()`, `get_parent_terragrunt_dir()`, `get_repo_root()`, `path_relative_to_include()`, `find_in_parent_folders()`, `abspath()`, `format()`, `join()` and literal locals resolve to exact files. A path that depends on the environment (`get_env`), dependency outputs, or another unevaluated value makes the unit depend on everything in its scope. Effect-dependent discovery still fails. This is not a claim that every Terragrunt expression has complete affected provenance. Results are sorted by stack, then by name within each component type.

## Scope

The fixtures cover local sources, local backend paths, includes, locals, nested stack values, literal generation, order and output dependencies, command-scoped mocks, fresh outputs, and native bulk execution. Unit tests cover absolute and relative workdir overrides, ownership conflicts, repeated provisioning without state loss, concurrent environment isolation, unit-relative child commands, cancellation, and rejected effects. The plugin workflow runs its unit tests on Linux, macOS, and Windows. These checks do not establish a compatibility percentage.

The host executes `extra_arguments` and `before_hook`, `after_hook`, and `error_hook` blocks with the invocation's environment, identity, toolchain PATH, and cancellation. The plugin only evaluates them. Hooks run once per Atmos phase, and an implicit `init` matches hooks for `init`, not for the requested command. Hook output goes to stderr. Workspace selection, `providers lock`, dependency-output reads, and `atmos terraform shell` do not run hooks. Hooks match the first word of the Terraform command, so `commands = ["state"]` runs for every `state` subcommand. A `plan -detailed-exitcode` status of 2 (changes present) is not a hook failure: after hooks run, error hooks do not, and Atmos still exits with 2. A relative hook `working_dir` or `required_var_files` entry resolves against the provisioned working directory; use `${get_terragrunt_dir()}` for paths beside `terragrunt.hcl`. A missing required var file fails before any process starts.

A saved plan is kept until it is applied. `output`, `validate`, `init` and `state list` leave it in place. A successful `apply`, `deploy` or `destroy` removes the plan and its Atmos record, so applying the same plan twice reports that no saved plan exists. Apply reuses the execution data recorded at plan time and requires the original working directory. The record holds no input values and no backend settings; Atmos compares the backend by digest at apply time.

Remote module sources (`git::https`, `git::http`, `git::ssh`, `git::file`, `https`, `http`) keep their getter URI for the Atmos source provisioner. A local module is copied into the working directory again only when its files change (path, size, or modification time); an explicit `source.ttl` takes precedence. A source that follows a branch (`?ref=main`) is fetched once and reused, and Atmos prints a notice when it reuses such a copy: run `atmos terraform source pull <component> -s <stack> --force` to refresh it, or set `source.ttl`. Release tags and commit ids are never reported. Registry sources and remote stack catalogs are not supported. Backend configuration is adopted, including cloud backends, but Atmos does not provision the backend resources.

Atmos removes a generated file when its `generate` block is removed, but only while the file is unchanged or still carries the Terragrunt signature. `if_exists = "error"` accepts the file Atmos generated on an earlier run, and all generated files are mode 0600. A `generate` block whose contents declare a `backend` or `cloud` block replaces the backend file Atmos would write, with or without `remote_state`. The `.terraform.lock.hcl` beside `terragrunt.hcl` is copied into the working directory when it changes; Atmos does not copy changes back, so copy `.workdir/.../.terraform.lock.hcl` yourself after `init -upgrade`.

A unit without a stable `remote_state` path keeps local state under `.terragrunt-cache`, which moves with the checkout path and disappears when the cache is deleted. Atmos keeps that location, to preserve existing state identity, and warns when it applies.

`dependencies.tools` is read from stack manifests and component sections, not from `atmos.yaml`. Units have no stack manifest, so a unit takes its tool pins from `.tool-versions`.

Unsupported semantics fail before any infrastructure command and never switch executors. Currently rejected: `terraform_binary`; `errors`, `exclude`, feature flags, and IAM role or web-identity settings; `remote_state` with `disable_init` or encryption; inline modules without `terraform.source`; output dependencies with a non-default workspace during discovery; the `read-config`, `terragrunt-read-config`, and `init-from-module` hook lifecycles; hooks that run a built-in integration such as `tflint`; and an invalid `on_errors` expression. Executable mode, typed provider flags and help, exact cty types and sensitivity, scoped Terragrunt logging, migration and setup generation, and Atmos Pro integration remain in the PRDs.

Unsupported settings fail with an error that names the unit, the file with line and column, and the setting. A broken unit does not affect other units or YAML components: it stays listed, and only commands that target it fail. `apply`, `plan`, and `destroy` with `--all` validate every selected unit first and fail with "no infrastructure command was run". Evaluation errors can contain secrets, so Atmos shows the file, line, unit, and a hint but not the evaluator's text; run with `ATMOS_LOGS_LEVEL=Debug` to log the full message with secrets masked.

Plugin settings are validated when first used, not when `atmos.yaml` loads, so `atmos config set` can always repair them. Unknown keys under `components.terraform.terragrunt` and invalid `mode` values fail with the key name. With `discovery.enabled: true` and no `included_paths`, discovery covers every unit under the project root. Only enabled/mode/provider/discovery settings are accepted by this package. Additional plugin settings and typed CLI option handling remain planned. Discovery that needs command execution, decryption, network access, or identity-dependent effects fails. The native plugin is trusted code, not an OS sandbox.

## Maintained upstream facade

The package pins Terragrunt v1.2.1. Some required upstream facilities are in `internal` packages. The local `bridge/` module uses the import path `github.com/gruntwork-io/terragrunt/pkg/atmosbridge`, which satisfies Go's internal-package import rule. This is an Atmos-maintained facade, not a public upstream Go API or an upstream-published module. A checked-in relative module replacement resolves it to this package's source.

The plugin also uses this checkout's SDK through a relative module replacement. Normal Go module commands work without temporary source patches or module-cache changes. Go 1.27.3 is selected by the plugin module:

```shell
go -C plugins/terragrunt test ./...
go -C plugins/terragrunt/bridge test ./...
```

See [the Terragrunt PRD](../../docs/prd/terragrunt-support.md) and [the shared component contract](../../docs/prd/plugins/extensions/component-engines.md) for the release requirements.
