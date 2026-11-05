# Skill extensions

**Status:** Follow-on adapter; contracts retained in the first-release design. [All extension types](README.md).

Plugins can ship skill instructions and supporting files alongside executable extensions, or as a data-only package. Reuse the existing [skill registry](../../../../pkg/ai/skills/registry.go), [loader](../../../../pkg/ai/skills/loader.go), and [client-distribution machinery](../../../../pkg/ai/skills/marketplace/installer.go).

## Consumer experience

```shell
atmos plugin install acme/okta --as okta --version '~1.0'
atmos ai skill list
# Proposed plugin-backed source syntax for existing skill installation:
atmos ai skill install plugin://okta/okta-operations --client claude
```

Plugin installation makes declared skills available to Atmos's effective project skill loader. Explicit client distribution selects where instructions are materialized; it does not download or install a second version of the plugin. `plugin info okta` lists skills and their ownership/version.

The `plugin://` source adapter resolves the installed alias and skill export at the plugin lock's version. Existing marketplace/project skill sources remain valid. Plugin-backed skills cannot independently upgrade to a different package version.

## Authoring and contract

```shell
atmos plugin create acme-guidance --capability skill --runtime data
```

The scaffold includes `SKILL.md`, metadata, package-local references, a sample consumer, structural fixtures, and a release workflow. A combined Go/Starlark package includes the same files under a declared skill directory.

- Manifest exports identify a skill's name, description, entrypoint, and required extension/tool references. Skill contents and supporting files belong to the verified package.
- Reuse supported skill format validation. Package-local paths cannot escape the package; referenced code/resources are included rather than fetched during skill loading.
- Skill availability is distinct from selecting/invoking it. Loading/listing metadata does not run included scripts or prepend every installed skill to every prompt.
- Declared tools and restrictions are evaluated against Atmos policy. Skill prose cannot grant permissions or bypass approval, identity, or secret handling.
- Resolve skill-name collisions explicitly; do not silently replace project-authored skills.
- Client copies carry ownership records tying them to package digest/version. Existing explicit sync/distribution operations reconcile updates and rollback; locally edited files require a reported conflict rather than destructive overwrite.
- Plugin removal reconciles owned skill registrations/copies and leaves unrelated or modified user files intact. Stale copies are diagnosed with recovery instructions.
- Consumers without external-client distribution still use the skill through Atmos's loader.

## Integration

| Aspect | Today |
|---|---|
| Registry | Instance-based [`skills.Registry`](../../../../pkg/ai/skills/registry.go), populated by [`LoadSkills`](../../../../pkg/ai/skills/loader.go). |
| Duplicate policy | `Register(*Skill) error` errors on duplicates; `LoadSkills` skips duplicates with a warning, so the first loaded wins. |
| Context on the interface | Not applicable; skills are data. |
| Reachable from outside the module | Yes. |
| Closed lists to open | The `LoadSkills` order: project skills from `.atmos/skills/installations.json` ([`owned_loader.go`](../../../../pkg/ai/skills/marketplace/owned_loader.go)), then `~/.atmos/skills` and the local registry, then inline `ai.skills` entries. There is no plugin stage or `plugin://` source. |
| Boundary leaks | None. `SKILL.md` frontmatter is [`marketplace.SkillMetadata`](../../../../pkg/ai/skills/marketplace/metadata.go); client distribution lives in [`marketplace/installer.go`](../../../../pkg/ai/skills/marketplace/installer.go). |

**Adapter:** `pkg/plugin/adapter/skill` is a loader stage, not a wire client. Skills are data inside the verified package, so a data-only package launches no plugin process and no wire service is called.

**Injection points:**
- `LoadSkills`: add a plugin-owned loader stage after project and user skills, with ownership tied to the package digest. Because the first loaded wins, reject and report plugin name collisions before registration so project-authored skills retain precedence.
- Skill source resolution: add a `plugin://<alias>/<skill>` source adapter that resolves at the plugin lock's version.
- `installations.json`: record ownership of client copies by package digest and version.

**Prerequisites:** none in Phase 0; Phase 3 work package WP3.8 (MCP and skills). See the [gap analysis](../platform/gap-analysis.md) and [delivery plan](../platform/delivery-plan.md).

**Must not cross the wire:** none. Skill content and metadata are read from the installed package by Atmos; no internal Go type reaches a plugin.

## Acceptance

A combined package exposes commands, an identity, an MCP tool, and a skill with one lock entry. Data-only skill installation launches no plugin process. Tests verify metadata, references, unavailable required tools, collision handling, Atmos policy, project/global isolation, owned client copies, edited-file conflicts, update, rollback, and removal. Structural/contract tests validate packaging and bindings; they do not claim deterministic correctness of model behavior.
