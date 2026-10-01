# Atmos Skill Development Framework (`atmos ai skill` spec-driven authoring)

**Status**: Proposed **Last Updated**: 2026-10-01 **Owners**: Atmos AI subsystem

**References**:

- [Spec-Driven Development: AI-native engineering](https://developer.microsoft.com/blog/spec-driven-development-ai-native-engineering/) (Microsoft)
- [Spec-Driven Development with Spec Kit](https://developer.microsoft.com/blog/spec-driven-development-spec-kit/) (Microsoft)
- [GitHub Spec Kit](https://github.com/github/spec-kit) - the open-source toolkit this framework is modeled on
- [Agent Skills Standard](https://agentskills.io) - the `SKILL.md` format (frontmatter, body, references)

**Related Atmos PRDs**:

- `docs/prd/atmos-agent-skills.md` (the skill format, install/list/marketplace this builds on)
- `docs/prd/atmos-ai.md` and `docs/prd/atmos-ai-local-providers.md` (the `atmos ai` command group and AI providers)
- `docs/prd/atmos-ai-global-flag.md` (`--ai` / `--skill` global flags that dev skills use immediately)

---

## 1. Executive Summary

### Problem

Authoring a good Atmos skill (a `SKILL.md` system prompt the AI interprets) is slow, unguided trial-and-error. A skill author must understand the domain, write a precise prompt that yields consistent high-quality output, test it across diverse scenarios (components, stacks, providers, error cases), iterate on wording where small changes swing behavior, and confirm it works across multiple AI providers (Anthropic, OpenAI, Gemini, Ollama). Today there is no framework, no guardrails, and no feedback loop - authors hand-write `SKILL.md`, eyeball a couple of runs, and hope. The result is inconsistent quality and no way to catch regressions when a prompt is edited.

A `SKILL.md` is, in effect, a specification that an AI must interpret correctly. That makes skill authoring a natural fit for Spec-Driven Development (SDD): *define the what and why before the how*, and *establish really good context before producing output*.

### Solution

Add a native spec-driven skill-development workflow to the existing `atmos ai skill` command group - no external dependencies. Atmos provides the full loop to create, generate, evaluate, validate, test, and publish skills, modeled on [GitHub Spec Kit](https://github.com/github/spec-kit)'s SDD methodology but implemented natively and specialized for `SKILL.md` authoring.

The workflow:

```
atmos ai skill create <name>    # Interactive wizard  -> .dev/spec.md (+ constitution)
atmos ai skill plan <name>      # AI: spec            -> .dev/plan.md
atmos ai skill generate <name>  # AI: spec + plan     -> SKILL.md + fixtures/ + evals/
atmos ai skill eval <name>      # Run evals (AI-as-judge), scores + suggestions (iterate)
atmos ai skill validate <name>  # Cross-artifact + constitution compliance check
atmos ai skill test <name>      # Live run against real atmos commands
atmos ai skill publish <name>   # Package + open PR to a skills marketplace
```

Because dev skills live where `--ai --skill` already looks, an author can test a skill against real commands the moment it is generated, and the eval loop gives a score-driven signal to iterate against instead of vibes.

## 2. Goals and non-goals

### Goals

- A guided `spec -> plan -> generate -> eval -> validate -> test -> publish` pipeline for `SKILL.md` authoring, native to `atmos ai skill`.
- Automated, reproducible quality measurement via **AI-as-judge** evals scored against an explicit expected-behavior checklist.
- **Multi-provider** eval matrix so a skill is verified to work across providers, not just the author's default.
- An auto-generated **constitution** encoding Atmos skill standards (the non-negotiable principles every skill must satisfy).
- Quality **gates before publish** (minimum eval/validation scores, provider coverage, eval count).
- Publishing to the official **Atmos Skills Marketplace** (`cloudposse/atmos` `agent-skills/skills/`), a custom GitHub repo, or local-only.
- Clean separation of development artifacts (never published) from the published skill (`SKILL.md` + `references/`).

### Non-goals

- A general-purpose SDD engine for arbitrary code (this is scoped to `SKILL.md` authoring; the repo's `speckit-*` skills cover code SDD).
- Replacing or forking the existing `agent-skills/skills/` format - published output must match existing skills exactly.
- Training/fine-tuning models; this orchestrates prompt-level skill authoring only.
- A hosted eval service - evals run locally (or in CI) against configured providers.

## 3. Methodology: Spec-Driven Development mapped to skill authoring

Spec Kit's phases map onto skill authoring as follows (Atmos implements these natively, not by shelling out to Spec Kit):

| Spec Kit phase | Atmos skill phase | Artifact |
|---|---|---|
| Constitution | Atmos skill standards (auto-generated) | `.dev/constitution.md` |
| Specify | The "what/why" of the skill (its PRD) | `.dev/spec.md` |
| Plan | The "how": prompt structure, examples, scenarios | `.dev/plan.md` |
| Tasks | Authoring checklist | `.dev/tasks.md` |
| Implement | Generate `SKILL.md` + fixtures + evals | `SKILL.md`, `fixtures/`, `evals/` |
| Analyze / Checklist | Validate cross-artifact consistency + standards | validation report |
| (Spec Kit converge) | Eval loop until score threshold met | `results/` |

Core insight carried over from SDD: *to produce the right output you must first establish really good context.* For a skill, the "context" is the spec + plan + constitution, and the generated `SKILL.md` is only as good as those.

## 4. CLI command surface

All under the existing `atmos ai skill` group (which already provides `install`, `list`, `update`, `uninstall`). New subcommands:

- **`create <name>`** - interactive wizard (domain, audience, behavior, key scenarios, constraints) that scaffolds the dev workspace and writes `.dev/spec.md` + `.dev/constitution.md`.
- **`plan <name>`** - AI analyzes `spec.md` and writes `.dev/plan.md` (prompt sections, scenarios, fixture/eval inventory).
- **`generate <name>`** - AI uses spec + plan to write `SKILL.md`, `fixtures/`, and `evals/`.
- **`eval <name>`** - runs each eval against configured providers using AI-as-judge; prints a per-eval score table + suggestions; writes `results/<timestamp>/`. Flags: `--providers`, `--min-score`, `--eval <name>`.
- **`validate <name>`** - checks `SKILL.md` against spec, plan, and constitution (coverage, format, error handling, caveats); prints a validation score. Flag: `--min-score`.
- **`test <name>`** - interactive live test: pick real `atmos ... --ai --skill <name>` commands to run and inspect output.
- **`publish <name>`** - runs pre-publish gates, then publishes to a selected marketplace (official / custom repo / local).
- **`fixture capture|create <name>`** - capture a fixture from a real command, or scaffold one manually.
- Later: **`clarify <name>`** and **`checklist <name>`** (Spec Kit clarify/checklist analogs).

The full happy path and example output for each command are specified in the design source (`atmos-skill-development-framework.md`); this PRD is the authoritative scope and may refine wording during implementation.

## 5. Directory structure

**Development workspace** (local, under `~/.atmos/skills/<name>/`):

```
<name>/
├── .dev/           constitution.md, spec.md, plan.md, tasks.md   (never published)
├── SKILL.md        the skill (generated, then refined)           (published)
├── references/     supporting docs the AI can reference          (published)
├── evals/          AI-as-judge eval cases                        (never published)
├── fixtures/       captured sample command outputs               (never published)
└── results/        eval run results (<timestamp>/summary.json)   (never published)
```

**Published skill** (in a marketplace repo, e.g. `agent-skills/skills/<name>/`): only `SKILL.md` + `references/`, matching the structure of existing skills (`atmos-terraform`, `atmos-stacks`, ...).

## 6. Evaluation framework

- **Eval file format**: markdown with `## Input` (what the AI receives as command output under `--ai --skill`), `## Expected Behavior` (a checklist), and `## Scoring Criteria` (weighted rubric).
- **AI-as-judge**: a separate AI call scores the skill's output against the expected-behavior checklist, producing a 0.0-1.0 score + notes. This enables automated regression testing on `SKILL.md` edits, cross-provider comparison, and CI quality gates.
- **Fixtures**: captured from real Atmos commands (`atmos ai skill fixture capture ... --command "atmos terraform plan vpc -s ..."`) or created manually, so evals are reproducible and require **no cloud credentials**.
- **Multi-provider matrix**: the same evals run across configured providers (Anthropic, OpenAI, ...), surfacing provider-specific weaknesses.

## 7. Constitution: Atmos skill standards

`create` auto-generates `.dev/constitution.md` from Atmos conventions. Non-negotiable principles include: no cloud credentials required for testing; provider-agnostic output; structured output (tables/sections, not free prose); actionable guidance (next steps on success, root-cause + fix on failure); explicit scope boundaries (decline out-of-domain rather than hallucinate); honest caveats; and adherence to Atmos naming/stack/component conventions. It also fixes output-format standards and the canonical `SKILL.md` section order. `validate` enforces the constitution.

## 8. Publishing and marketplace

`publish` runs pre-publish gates (e.g. eval score >= 0.80, validation score >= 85, >= 4 eval cases, >= 2 providers tested) and then targets one of:

1. **Atmos Skills Marketplace** (official): fork `cloudposse/atmos` if needed, branch, copy `SKILL.md` + `references/` into `agent-skills/skills/<name>/`, and open a PR (with description, eval results, metadata) for Cloud Posse review. Once merged, installable via `atmos ai skill install <name>`.
2. **Custom GitHub repo** (e.g. `acme-corp/atmos-skills`): branch + PR into that repo's `skills/<name>/`; installable via `atmos ai skill install <owner>/<repo>/<name>`.
3. **Local only**: no publish; usable immediately via `--ai --skill <name>`.

## 9. Integration points

- **Existing**: `atmos ai skill install` / `list` (dev skills appear as installed), and the `--ai` / `--skill` global flags - dev skills work during development with zero extra wiring.
- **AI providers**: reuse the existing `atmos ai` provider layer (Anthropic/OpenAI/Gemini/Ollama + OpenAI-compatible) for both generation and AI-as-judge; honor configured provider selection and auth.
- **CI/CD**: `atmos ai skill validate <name> --min-score 85` and `atmos ai skill eval <name> --min-score 0.80 --providers anthropic,openai` as PR quality gates.

## 10. Architecture and implementation notes (Go)

Following repo mandates:

- **Command registry**: each new subcommand registers via the `CommandProvider` pattern under `cmd/ai/skill/` (mirrors existing `install`/`list`/`update`/`uninstall`); flags via `flags.NewStandardParser()` (never direct `viper.BindEnv`/`BindPFlag`).
- **Business logic** in `internal/exec/` (or a new `pkg/ai/skill/` package) behind interfaces; AI/provider calls and the AI-as-judge scorer are injection seams so the pipeline is unit-testable with mocked providers (no live API in unit tests).
- **Options pattern** for pipeline configuration; `perf.Track` on public functions; static error sentinels in `errors/errors.go`; `data.*`/`ui.*` for output (tables via theme-aware helpers), never `fmt.Print*`.
- **Schemas**: any new `atmos.yaml` config (e.g. `ai.skill` dev settings) updates `pkg/datafetcher/schema/` and docs.
- **New purpose-built package(s)** rather than growing `pkg/utils`.

## 11. Phased implementation

- **Phase 1 - Foundation**: `create` (wizard), `generate` (SKILL.md from answers), constitution template, dev directory layout.
- **Phase 2 - Evaluation**: eval file format + runner, AI-as-judge scoring, `eval`, fixture capture.
- **Phase 3 - Validation and testing**: `validate` (spec/constitution compliance), `test` (live), multi-provider matrix, score-based publish gates.
- **Phase 4 - Full workflow**: `plan` (AI technical plan), `publish` (marketplace), CI/CD integration, `clarify` + `checklist`.

Each phase is a separate PR with tests, docs, and (for user-visible features) a changelog + roadmap entry per the repo's PR policy.

## 12. Acceptance criteria

- `atmos ai skill create <name>` scaffolds the dev workspace with `spec.md` + `constitution.md`; `generate` produces a valid `SKILL.md` (matching the published format) plus starter fixtures and evals.
- `eval` runs the eval set against >= 2 providers, prints per-eval scores via AI-as-judge, and writes `results/<timestamp>/`; re-running after a `SKILL.md` edit reflects the change (regression signal).
- `validate` reports spec coverage and constitution compliance with an overall score and actionable fixes.
- `test` runs a selected real `--ai --skill` command and surfaces the skill's output.
- `publish` enforces the pre-publish gates and, for the official marketplace, opens a PR adding `agent-skills/skills/<name>/` (`SKILL.md` + `references/` only).
- Development artifacts (`.dev/`, `evals/`, `fixtures/`, `results/`) are never included when publishing.
- The whole create -> generate -> eval loop runs with **no cloud credentials** (fixtures + mock components).

## 13. Open questions

- **Config surface**: what belongs under `atmos.yaml` `ai.skill` (default providers for eval, score thresholds, workspace path) vs per-command flags?
- **Eval determinism**: how to keep AI-as-judge scores stable enough for CI gates (fixed judge model/temperature, score banding, multiple samples)?
- **Provider cost/caps**: eval matrices multiply API calls; need guardrails (provider allowlist, `--eval` scoping, caching).
- **Workspace location**: `~/.atmos/skills/` vs an XDG path vs in-repo `.atmos/skills/` for project-scoped skills.
- **Publish auth**: reuse `github/sts` / `gh` token resolution for the fork + PR flow.
- **Relationship to the repo `speckit-*` skills**: shared vocabulary, but this is a distinct, purpose-built pipeline - confirm no overlap/confusion in naming.
