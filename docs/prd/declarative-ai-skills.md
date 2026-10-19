# Declarative AI Skill Sources

## Purpose

Projects declare skill sources and reproducible selections independently from installing them.
Configuration CRUD (`add`, `set`, `remove`) never downloads or modifies installed files.
Installation CRUD (`install`, `update`, `uninstall`) and reconciliation (`sync`) own that work.

## Configuration and resolution

Source declarations extend `ai.skills` without replacing inline entries. Sources select a repository
or local directory, optional ref/subpath, marketplace plugins, name filters, clients, and scope.
The `!version` tag is accepted only at a source's `ref` in Atmos configuration. Its dependency identity
survives scalar merges and is resolved lazily against the selected version lock during installation.
Configuration editing and version-lock bootstrapping do not require that dependency to be locked.

The portable `skills.lock.yaml` partitions resolutions by track and source label. Each resolution
records the declaration, managed version, independent repository commits, selected skill paths,
and content digests. Unchanged declarations reuse resolutions; mutable literal refs advance only
on update. Managed refs retain matching resolutions until their version lock or declaration changes.
Local source changes require explicit update and never receive invented Git commits.

Marketplace support is limited to relative, GitHub, Git URL, and Git-subdirectory skill sources.
Unsupported selected source types fail. Plugin hooks, agents, commands, MCP, dependency execution,
npm, and archives are excluded.

## Installed state

Project and user canonical trees and ownership records are distinct. Every destination records
project/source ownership, track, scope, client, path, and last-applied digest. Legacy global skills
remain readable, but legacy client destinations are not inferred from a matching name.

Project skills override user skills, which override inline entries. Shadowing emits a warning.
Chat startup never fetches sources. Ad-hoc resolutions are stored locally and are excluded from
ordinary declarative pruning.

## Reconciliation

A single resolver/planner/apply sequence stages complete skill trees, validates containment and
ownership, and replaces destinations by rename. Full replacement removes upstream-deleted files.
Explicit client flags restrict changes, retaining canonical content needed by other client copies.

Ordinary sync reports obsolete copies. Explicit pruning removes unchanged owned copies. Modified
owned content requires force; force never transfers ownership. Frozen mode prohibits lock changes.
Check mode is offline and mutation-free. Dry runs may use disposable download directories only.

Writers serialize project and user state in sorted lock order. A durable journal records staged
paths and backups before replacement. Metadata is committed after content. Apply failures roll
back; interrupted rollback retains the journal and backups and blocks new mutations. Explicit
recovery rolls back uncommitted transactions or cleans committed ones.

## Acceptance

Unit and CLI tests cover deferred tags, configuration-only edits, independent plugin commits,
local content changes, project isolation, user ownership conflicts, frozen replay, pruning,
client-scoped deletion, filesystem containment, rollback, recovery, and writer exclusion.
