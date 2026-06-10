---
name: docs
description: "Docs: contributor documentation conventions for Atmos website docs, CLI command docs, configuration pages, action cards, changelog, roadmap, and stale-content checks"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Docs

Use this skill when changing documentation for Atmos itself: website docs, CLI command docs, `atmos.yaml`
configuration docs, changelog posts, roadmap entries, and project-local contributor guidance.

## First Pass

Before editing, inspect the related implementation and existing docs:

```bash
rg -n "<feature>|<config-key>|<command>" website/docs docs agent-skills .claude/skills
rg -n "<feature>|<config-key>|<command>" pkg cmd internal
```

Search for stale claims before finishing:

```bash
rg -n "unsupported|not supported|not currently|not enforced|TODO|coming soon" website/docs docs agent-skills .claude/skills
```

## Configuration Docs

Every new or changed `atmos.yaml` section needs configuration docs.

- Add or update the parent page under `website/docs/cli/configuration/`.
- Add a child page when a nested section has independent behavior, policies, defaults, or command-facing effects.
- Keep parent pages as summaries when child pages exist; link to the child page for details.
- Use `<File title="atmos.yaml">` for config examples.
- Use `<dl>`, `<dt>`, and `<dd>` for configuration keys and option definitions.
- Include defaults, supported values, and environment variables when they are part of the public interface.

## Command Docs

When command behavior is configured by `atmos.yaml`, link command docs back to configuration docs.

- Import `ActionCard` and `PrimaryCTA`.
- Place the card near the top, after `Intro` and any status badges.
- Link to the relevant configuration page, not just the root docs section.
- Use definition lists for flags and positional arguments.
- Use `DocCardList` for command families and subcommands.

Example:

```mdx
<ActionCard title="Configure Toolchain">
    Learn how to configure tool versions, registries, aliases, and package verification in your atmos.yaml.
    <div>
      <PrimaryCTA to="/cli/configuration/toolchain">Configuration Reference</PrimaryCTA>
    </div>
</ActionCard>
```

## Changelog Posts

Changelog posts live in `website/blog/` as dated `.mdx` files. Follow the template and tag/author rules in
`CLAUDE.md` and the `pull-request` skill. In addition:

- **NEVER start a sentence, paragraph, heading body, list item, or the post intro with a backtick** (inline code
  span) or a code fence. Lead with prose, then introduce the code. Markdown that opens on a backtick reads
  awkwardly, breaks scannability, and can render poorly in feed/preview contexts.
  - Wrong: `` `atmos describe affected` now compares every section. ``
  - Right: ``The `atmos describe affected` command now compares every section.``
  - Wrong (list/intro): `` `vars`, `env`, `settings` are evaluated. ``
  - Right: ``The evaluated sections are `vars`, `env`, and `settings`.``
- Put a real code block (fenced, language-tagged) on its own lines when showing commands or config — not inline at
  the start of a sentence.

## Release Docs

When behavior changes, update all user-facing surfaces in the same PR:

- Configuration docs for new or changed `atmos.yaml` keys.
- Command docs for changed CLI behavior.
- Consumer agent skills when product behavior changes how AI assistants should answer questions about Atmos.
- Claude skills when contributor documentation workflows or repo-local development guidance changes.
- Changelog and roadmap pages when the feature is user-visible.
- Remove or revise stale “unsupported”, “not enforced”, and “not currently” language.

## Validation

Run the narrowest useful validation first, then broader checks if website or skills changed:

```bash
git diff --check
cd website && pnpm run build
```

For agent skills, mirror `.github/workflows/validate-agent-skills.yml`:

- each skill has a `SKILL.md`
- `SKILL.md` frontmatter has `name` and `description`
- `SKILL.md` stays under 500 lines and 20KB
- reference files stay under 25KB
- all code fences include language tags
