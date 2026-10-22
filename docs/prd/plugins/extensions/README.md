# Extension types

An **extension** connects plugin functionality to an Atmos surface. A **plugin** is the package users install and lock; it can export any combination of supported extension types. Authentication and executable commands ship first. The other PRDs define follow-on adapters; their extension mechanisms are part of the [first release](../first-release.md). [Platform overview](../overview.md) · [Package composition](../package-composition.md).

| Extension | Export selector | Contract |
|---|---|---|
| [Authentication](authentication.md) | `auth.provider`, `auth.identity` | Providers, identity chains, and credentials. |
| [Component engines](component-engines.md) | `component`, `ci.component` | Component lifecycle, execution, and component-specific reporting. |
| [CI providers](ci-providers.md) | `ci.provider` | CI platform detection, context, and reporting. |
| [Commands](commands.md) | `command` | Top-level commands and qualified dispatch. |
| [Validation](validation.md) | `validation.validator`, `validation.rulepack` | Policy, linting, schemas, and shared diagnostics. |
| [Hooks](hooks.md) | `hook` | Explicitly configured lifecycle callbacks. |
| [Steps](steps.md) | `step` | Reusable automation tasks. |
| [MCP tools](mcp-tools.md) | `mcp.tool` | Tools served through Atmos's MCP server. |
| [Skills](skills.md) | `skill` | Packaged instructions and supporting resources. |
| [Stores](stores.md) | `store` | Key/value backends. |
| [Backend provisioning](backend-provisioning.md) | `provisioner.backend` | State-backend infrastructure lifecycle. |
| [Secret providers](secret-providers.md) | `secret.provider` | Sensitive storage and retrieval with declared scope. |

Selectors are proposed values for `plugin create --capability` and catalog filtering. “Capability” remains the manifest's technical vocabulary for extension declarations and optional features. It does not imply separate packages.

Go/native protocol supports the first release. Starlark support and the remaining executable extensions follow later. Skills and rule packs can use the data runtime without compiled code. All use shared discovery, installation, locking, trust, and tests. [Other surfaces and follow-ons](../surface-area.md) are outside this list.
