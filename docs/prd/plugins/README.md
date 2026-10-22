# Atmos plugin PRDs

**Status:** Native Go pilot implemented in the working branch; not yet released. The [first-release checkpoint](first-release.md#native-pilot-checkpoint) distinguishes implemented behavior from remaining acceptance work and follow-on designs.

A **plugin** is the package users install, version, and lock. An **extension** is functionality it provides. One package can include commands, identities, MCP tools, skills, and other extensions together.

Start with the [first-release scope](first-release.md) and [overview](overview.md), then the [Okta package example](package-composition.md) and [extension index](extensions/README.md).

| Document | Contents |
|---|---|
| [First release](first-release.md) | Required commands, auth registry prerequisites, Go OIDC pilot, scaffolding reuse, and future extension mechanisms. |
| [OIDC pilot](oidc-pilot.md) | Complete Go browser-login/exchange/reuse/refresh/logout acceptance and supporting contracts. |
| [Overview](overview.md) | Scope, shared architecture, OCI catalogs/CRUD, compatibility, trust, and release gates. |
| [Package composition](package-composition.md) | One plugin with commands, auth, MCP tools, and skills. |
| [User experience](user-experience.md) | Discovery, installation, configuration, execution, update, and restoration. |
| [Authoring](authoring.md) | Generated boilerplate, Go/Starlark auth walkthrough, testing, and publication. |
| [Distribution](distribution.md) | Compiled platform artifacts, OCI indexes, and compiler-free installation. |
| [Discovery](discovery.md) | Optional catalog snapshots, direct repository indexing, shared search, caching, and catalog publisher coordination. |
| [Toolchain dependencies](toolchain-dependencies.md) | Required tools, scoped paths, plugin/tool locks, frozen restores, and rollback. |
| [Testing](testing.md) | Unit tests, mocked contract fixtures, runtime parity, and release verification. |
| [Extension PRDs](extensions/README.md) | A contract and acceptance criteria for each planned extension type. |
| [Automation contracts](automation-contracts.md) | Shared hook/step examples and Atmos execution behavior. |
| [Reference models](reference-models.md) | Helm/TFLint lessons and the custom-Atmos-build experience to exclude. |
| [Surface map](surface-area.md) | Additional integration points and defined follow-ons. |
| [Plugin policies](policy.md) | CEL/Starlark admission for registries, packages, cache reuse, development, and publication; signature enforcement and trust boundaries. |
| [Architecture policy](architecture-policy.md) | Default pattern for new integration features and criteria for core changes. |
| [Built-in migration](builtin-migration.md) | Future integration packages for Terraform, Helm, Ansible, and Kubernetes. |
| [Platform workstream](platform/README.md) | What has to be built: gap analysis, interfaces, protocol, configuration, errors, decisions, and the delivery plan. |

All extensions share the package lifecycle, catalog, lockfile, and runtime platform. Focused PRDs supplement the overview; auth and executable commands gate the first release; the remaining adapters and Starlark/data runtimes are follow-ons.

The documents above describe the product. The [platform workstream](platform/README.md) describes the engineering: the [gap analysis](platform/gap-analysis.md) inventories every registry that exists, is missing, or must change, and the [delivery plan](platform/delivery-plan.md) sequences that work. The auth registry refactor and shared first-release foundations are validated with the OIDC implementation in one working change, then split into reviewable PRs after validation. Other registry work accompanies the relevant adapters.
