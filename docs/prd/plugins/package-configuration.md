# PRD: Configuration from OCI plugin packages

**Status:** Proposed follow-on. Automatic package configuration and Starlark package execution are not implemented by the native auth/command pilot. All new manifest fields below are proposed. [All plugin PRDs](README.md).

## Problem and outcome

Organizations should be able to publish repository checks, custom commands, Git hooks, and their supporting scripts as one versioned OCI plugin. Consumers should not copy files, maintain includes, or know where Atmos caches the package.

The complete consumer declaration is:

```yaml
plugins:
  packages:
    quality:
      source: oci://ghcr.io/acme/repo-tools
      version: "^1.0"
```

After the package is resolved and admitted through the normal plugin lifecycle, Atmos automatically registers its declared configuration. For example, a packaged `check` command becomes `atmos quality check`. Packaged Git hook definitions become available to the existing Git hook runner. Creating Git shims still requires `atmos git hooks install`; loading configuration never installs hooks or runs checks.

One package may contain Go SDK services, configuration, and Starlark scripts together. Configuration is an additional capability, not a mutually exclusive runtime or a second package to install.

## Package contract

The package root contains `plugin.yaml` and, when explicitly exported, `atmos.yaml`:

```text
plugin.yaml
atmos.yaml
bin/repo-tools
commands/check.yaml
lib/checks.star
schemas/check-options.json
```

A proposed manifest fragment for a hybrid package:

```yaml
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/repo-tools
  capabilities:
    configuration:
      default:
        path: atmos.yaml
```

The manifest also declares whichever native auth, command, or [function](extensions/functions.md) exports the package supplies. The `configuration` descriptor negotiates a versioned contract through the existing compatibility mechanism. Hosts that do not support a required configuration contract reject the package before activation.

The manifest declaration is author opt-in; consumers need no extra include or enablement entry. A file named `atmos.yaml` alone does **not** opt a package in. Existing author repositories already use that filename for development commands such as build and push. These commands must never become consumer commands merely because an old package contains the file. Authors maintain their development configuration separately from the exported consumer entry point when packaging.

`plugin.yaml` owns package identity, compatibility, and capability declarations. Exported `atmos.yaml` uses the existing Atmos command and Git hook schemas within the supported contribution sections. It does not become a second source of plugin identity or installation policy.

## Automatic loading and precedence

Bootstrap establishes package selection before loading contributions, avoiding a dependency on plugin code:

1. Load project configuration using the existing configuration search and precedence rules. Establish package declarations, enabled state, registry authentication, trust, cache, and offline/frozen settings without evaluating plugin functions.
2. Resolve the selected releases and apply the existing compatibility and admission checks. Read each declared configuration entry point and its static includes, preserving source provenance.
3. Validate the complete contribution graph, supported sections, command paths, hook composition, and referenced package resources. No executable is started and no Starlark module is evaluated during this stage.
4. Compose package defaults, then apply project configuration. Retain the existing higher precedence of environment variables and CLI flags where those overrides are supported.
5. Publish the validated registrations together. Runtime dispatch starts code only when a command, hook, or function is invoked.

Package configuration cannot declare additional `plugins`, change registry authentication or trust, change project configuration discovery or base paths, or weaken policy. Initial loading permits static YAML and package-relative includes; executable YAML functions and templates that invoke code are rejected. Unsupported configuration sections produce a diagnostic naming the package, file, and key instead of being silently ignored.

The initial contribution adapters accept `commands` and `git.hooks`. Other surfaces, including workflow definitions, require their own explicit composition contract before activation. Packages may carry additional templates, schemas, and other resources, but carrying a file does not automatically register a new Atmos feature.

Inspection and help retain the existing lifecycle's read-only behavior: they use installed static metadata and do not start code or implicitly repair missing packages. Missing configuration payloads produce actionable restore diagnostics. Normal execution follows the configured installation policy; frozen/offline execution never silently changes the selected release.

## Command and hook composition

Package custom commands use the consumer alias as their command namespace: `check` in package `quality` registers as `atmos quality check`. Changing the alias changes that namespace without modifying the package. Commands retain their existing argument, flag, and step schema.

Built-in commands cannot be replaced. Project YAML commands take precedence over package defaults at the same complete command path. Duplicate package command leaves, including a native command and a YAML command claiming the same leaf, are invalid registrations. Existing native command shortcut collisions retain their qualified-dispatch behavior described in [command extensions](extensions/commands.md).

For a given Git event, package step-based hook contributions concatenate in lexicographic alias order; each contribution retains its declared step order and originating package. A project definition of that event replaces the combined package hook as a whole, including an explicit disabled definition. Command-based hook definitions cannot be concatenated: conflicting package definitions fail unless the project supplies a replacement. The loader must not rely on generic YAML list merging to determine execution order.

An installed package that only exports a lifecycle hook kind still does not subscribe to events. A configuration-capable package can additionally contribute explicit Git hook definitions. Both mechanisms use the existing hook runner and failure semantics; neither introduces package install/uninstall scripts.

## Paths, execution, and provenance

Every contributed node retains its package identity, selected digest, source file, and configuration key. Diagnostics and configuration inspection expose that provenance and explain project overrides.

Includes and script/resource references resolve relative to the originating package file. Local Starlark `load()` resolves relative to the importing module. All such references remain inside the admitted payload; traversal, absolute paths, symlink escapes, and include/load cycles are rejected. Package resources are read-only.

Resource location is separate from execution location. Commands run in the project execution context, and Git checks run in the staged sandbox when requested by the Git runner. Resolving a script from the package cache must not change the working directory to that cache. Package scripts use relative modules for shared implementation rather than hard-coding the consumer's alias.

Atmos keeps using its managed package cache. Consumers do not configure `path: .atmos/plugins/...`, copy resources into `atmos.d`, or include a cache path. Any project-local bookkeeping under `.atmos/plugins/` remains Atmos-managed, not a public dependency layout.

## Hybrid packages and release atomicity

A Go SDK plugin can export native services while its `atmos.yaml` contributes YAML commands and hooks backed by host-executed Starlark. A native package can also export functions implemented in Go or packaged Starlark under the [functions contract](extensions/functions.md). The runtime of a native service does not dictate the format of every resource in its package.

All parts share one identity, version, OCI release, trust decision, and lock selection. Installation, update, and rollback validate the complete package and cannot combine a binary from one release with configuration or scripts from another. Static contribution failure prevents activation; partial runtime registration must also be rolled back.

Hybrid packages use the existing native platform index and OS/architecture/variant selection. Common configuration and scripts must be identical across platform payloads and verified by packing. Consumers download only the selected platform payload and required metadata, not every architecture. A resource-only change still produces a new package release/digest. Shared resource layers are a possible storage optimization, not a requirement for this contract.

Configuration-only and Starlark-only packages require the planned platform-independent artifact/runtime support. They must not pretend to be native binaries or advertise a fabricated host architecture to pass current validation. Supporting hybrid packages must not depend on consumers installing a Go compiler.

## Integration and delivery

Add a `configuration` capability descriptor and contribution adapters through the existing plugin registry. Extend pack/validation to include the declared entry point and its resources, preserve provenance through the configuration loader, and register contributions before command selection and hook lookup. Do not introduce a second installer or global mutable registration state shared across projects.

Reuse the repository-check and staged-sandbox work in [PR #3494](https://github.com/cloudposse/atmos/pull/3494), whose merge status is independent of this proposal. This PRD defines distribution and registration, not a competing check framework. CLI commands and Git hooks should invoke the same packaged check implementation, retaining that framework's handling of staged and unstaged changes.

Implementation proceeds through static contribution validation and precedence, hybrid packaging, command/hook registration, and finally platform-independent packages. Each phase must identify its supported contracts rather than documenting proposed syntax as available functionality.

## Acceptance criteria

- A consumer declares one OCI package and runs its custom command with no manual include, `load()`, copied file, or cache path.
- One release provides a working Go SDK service and packaged configuration/scripts; both are activated, updated, and rolled back together.
- Renaming or disabling an alias consistently changes/removes its registrations. Two projects using different releases do not share configuration or execution state.
- Existing native packages containing an undeclared authoring `atmos.yaml` behave exactly as before.
- Tests cover project overrides, deterministic multi-package hook order, native/YAML command collisions, unsupported sections, and attempted bootstrap-policy overrides.
- Traversal, symlink escapes, include/load cycles, missing resources, and invalid schemas fail before code runs, with source provenance.
- Help and inspection do not execute native services, Starlark top-level code, check steps, or Git shim installation.
- The same packaged checks work through a custom command and a Git hook; staged execution preserves unrelated index and working-tree changes.
- Offline/frozen restores, compatibility rejection, failed activation, and update rollback retain the existing lock and trust guarantees.
- Native platform/variant tests verify identical shared resources and selection of only the host payload. Platform-independent packages have separate contract tests.
- Implementation achieves at least 90% authored patch coverage; generated bindings are reported separately.
