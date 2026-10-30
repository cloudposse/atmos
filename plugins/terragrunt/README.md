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

The default suite runs Go unit tests and Starlark affected-selection fixtures. It requires Go and Git, but no Python, shell, Terraform, or Terragrunt executable. The integration suite requires OpenTofu or Terraform on PATH. It copies both fixtures into temporary directories, applies only built-in `terraform_data` resources with local state, checks unchanged plans and dependency refresh, then destroys those resources. Deferred cleanup removes the temporary directories even when an assertion fails. Neither suite calls the Terragrunt CLI.

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

Source tracking currently covers files read during partial discovery, declared HCL, local module directories, and stack template provenance. Reads in unevaluated input expressions, arbitrary dynamic globs, and effect-dependent discovery need further work. This is not a claim that every Terragrunt expression has complete affected provenance.

## Scope

The fixtures cover local sources, local backend paths, includes, locals, nested stack values, literal generation, order and output dependencies, command-scoped mocks, fresh outputs, and native bulk execution. Unit tests cover absolute and relative workdir overrides, ownership conflicts, repeated provisioning without state loss, concurrent environment isolation, unit-relative child commands, cancellation, and rejected effects. The plugin workflow runs its unit tests on Linux, macOS, and Windows. These checks do not establish a compatibility percentage.

Current limits include remote module/catalog sources, non-local backends, Terragrunt hooks and extra arguments, IAM options, feature flags, error/retry blocks, prevention/exclusion rules, inline modules without `terraform.source`, named-workspace output dependencies, and saved-plan apply. Unsupported semantics must fail rather than switch executors. Explicit `terragrunt://` imports, state-function host dispatch, executable mode, engine table columns, UI/log adapters, and conversion commands remain in the PRDs.

Only enabled/mode/provider/discovery settings are accepted by this package. Additional plugin settings and typed CLI option handling remain planned. Discovery that needs command execution, decryption, network access, or identity-dependent effects fails. The native plugin is trusted code, not an OS sandbox.

## Maintained upstream facade

The package pins Terragrunt v1.2.1. Some required upstream facilities are in `internal` packages. The local `bridge/` module uses the import path `github.com/gruntwork-io/terragrunt/pkg/atmosbridge`, which satisfies Go's internal-package import rule. This is an Atmos-maintained facade, not a public upstream Go API or an upstream-published module. A checked-in relative module replacement resolves it to this package's source.

The plugin also uses this checkout's SDK through a relative module replacement. Normal Go module commands work without temporary source patches or module-cache changes. Go 1.27.3 is selected by the plugin module:

```shell
go -C plugins/terragrunt test ./...
go -C plugins/terragrunt/bridge test ./...
```

See [the Terragrunt PRD](../../docs/prd/terragrunt-support.md) and [the shared component contract](../../docs/prd/plugins/extensions/component-engines.md) for the release requirements.
