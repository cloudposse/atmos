# Extension contracts as the default development pattern

**Status:** Required architectural direction for the plugin platform and subsequent Atmos features. [All plugin PRDs](README.md).

## Default

New integration functionality should use the established extension contracts and registries. A package can supply several extensions and share its implementation. Once the platform is available, independently distributable integrations should normally ship as plugin packages.

During platform development, compiled-in implementations may exercise the same SDK interfaces and contract fixtures. They must be described accurately as compiled-in; registry registration alone does not make them independently installable.

## Design requirements for new features

1. Identify the existing extension type or combination that fits. Prefer a command, step, hook, validator, or another established type before inventing a new lifecycle.
2. Define static metadata and configuration/result schemas. Metadata must support discovery, validation, and help without executing the implementation.
3. Use the public versioned contract and API services. Do not pass internal managers, mutable Go structs, callbacks, or undocumented environment conventions across the plugin boundary.
4. Keep business/tool-specific behavior in the extension. Atmos owns shared orchestration, auth policy, secret masking, scheduling, output, trust, and dependency restoration.
5. Declare tool dependencies, platforms, runtime/API requirements, and optional operations. Supply precompiled native artifacts and lockable selections; users must not build a custom Atmos.
6. Include author fixtures and SDK contract tests. Exercise both compiled-in and external adapters where the feature has both; cover cancellation, error classification, isolation, and backward compatibility.
7. Make schemas, scaffolds, documentation, and compatibility tests part of delivering a public extension API. Additive optional functionality must not require upgrades to unrelated extensions.
8. Apply the shared [plugin admission policy](policy.md) before downloading or activating a package. New adapters use the same extension registry to identify capabilities; they must not create a separate registry allowlist or skip current policy on cached artifacts.
9. Keep CLI wiring thin. Lifecycle orchestration belongs in `pkg/plugin/manage`, installation in `pkg/plugin/install`, transport in `pkg/oci`, and presentation in `pkg/plugin/output` using shared UI packages. Add no plugin business logic to `internal/exec`; the long-term direction is to retire that generic package.

Feature PRDs should state the chosen extension type(s), package boundary, activation/configuration, API-service additions, compatibility, and tests. A new extension type requires a focused PRD under `extensions/` and an update to the index.

## When core changes are appropriate

Core remains responsible for configuration/bootstrap, stack composition/provenance, dependency graphs, shared execution services, protocol/runtime management, and security policy. A feature may require a core primitive that several extensions then use.

A core implementation is also appropriate where measured performance or a bootstrap dependency prevents a reasonable extension boundary. Record that constraint and the smallest core addition needed. The goal is consistent contracts and independent delivery where practical, not an RPC call for every internal helper.

Internal registries do not automatically become public APIs. Versioning, trust, input lifetime, concurrency, errors, and Atmos support must be designed before exposing one. New integrations must not bypass the architecture with hardcoded provider switches or an alternate plugin/package manager.

## Review and migration

Review new integration work against this pattern. Require a stated reason for keeping integration-specific functionality in core or introducing another extension point. Preserve existing behavior while replacing closed lists with registered metadata.

The [built-in migration plan](builtin-migration.md) applies the same discipline to Terraform, Helm, Ansible, Kubernetes, and later integrations. Extraction is staged after parity and compatibility tests; this policy does not require an immediate rewrite of all built-ins.
