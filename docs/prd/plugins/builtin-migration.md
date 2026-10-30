# Migrating integrations into plugin packages

**Status:** Intended direction after the initial external-plugin release; extraction is not an initial-release requirement. [All plugin PRDs](README.md).

## Goal

Integrations such as Terraform, Helm, Ansible, and Kubernetes should eventually ship as independently versioned packages. Each can provide commands, a component engine, hooks, validators, CI reporting, MCP tools, and skills together.

The extraction sequence and each integration's current implementation require an audit. Names here describe target package boundaries, not a claim that every engine already exists as a built-in.

## Responsibilities

| Atmos | Integration package |
|---|---|
| CLI bootstrap, configuration, stack composition and provenance | Tool-specific commands, schemas, flags, and component behavior |
| Installation, trust, locking, runtime management | Compiled platform artifacts or packaged Starlark/data |
| Auth orchestration and scoped credentials | Credential requirements and integration-specific behavior |
| Toolchain resolution and executable paths | Tool dependencies and invocation logic |
| Scheduling, events, diagnostics and CI services | Lifecycle callbacks and plan/result interpretation |
| MCP server and skill delivery | Integration-specific tools and guidance |

Charts, modules, playbooks, and Kubernetes manifests remain infrastructure content. Executable plugin packaging does not replace content vendoring.

## Migration gates

1. Validate new external integrations first, including a package with several extension types.
2. Audit internal dependencies and supply missing public contracts before extraction.
3. Prove parity for existing configuration, arguments, help, exits, authentication, plan/apply behavior, hooks, validation, and CI output.
4. Choose default distribution explicitly. Standard distributions may ship a tested set of separately packaged integration artifacts; minimal distributions may require explicit installation. Bundled packages remain independently versioned.
5. During an announced migration, a Atmos-owned mapping may recognize an official replacement for a built-in. Never register both under one name; arbitrary plugins still cannot replace built-ins.
6. Preserve project precedence and locks. Ordinary commands never fetch missing packages or modify locks. Missing artifacts identify the restore command; project selections cannot silently fall back to bundled versions.
7. Publish an Atmos/package compatibility matrix and rollback path before removing the compiled-in implementation.

The initial platform must enable this direction. Default bundles, extraction order, and release cadence remain follow-on decisions; additional interfaces such as CI caches require their own contracts where necessary.
