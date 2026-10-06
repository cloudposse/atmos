# Fix: Give Atmos Automation Language a documentation home

**Date:** 2026-10-04

## Summary

Make `/automation` the entry point for workflows, custom commands, lifecycle
hooks, standalone CLI apps, and Atmos Automation Language, alongside Atmos AI
and Native CI in the documentation sidebar.

## Context

The scripting reference explained individual APIs, but users needed a place to
understand what they could automate and choose a starting point.

## Changes

- Explain why Atmos uses Starlark in the Automation overview, language reference,
  and both announcements: a deliberately small shared language, familiar syntax,
  repeatable pure logic, explicit host capabilities, and an included runtime.
  Connect the design choice to HCL's focus without claiming that scripts with
  external effects are deterministic or safe to execute as untrusted code.

- Introduce the Python-like language with a runnable shebang quick start.
- Embed focused recordings for executable scripts, custom commands, and lifecycle
  hooks, with links to their complete runnable examples.
- Explain how shared functions, parallel tasks, workflows, and CI build on those
  starting points, and link to the existing detailed reference.
- Link the changelog introduction to the new page and add sidebar navigation.
- Route the homepage's Workflows & Automation card to `/automation` and describe
  the broader set of automation capabilities.
- Lead the overview with what users can build, then explain each entry point.
  Distinguish custom Atmos subcommands (`atmos capacity`) from standalone CLI
  apps (`./capacity.star`), while showing how both use the embedded language.
- Replace sidebar links to example pages with documentation pages for the
  language, standalone CLI apps, custom commands, workflows, lifecycle hooks,
  and testing.
  Keep primary overview links in the documentation and examples as optional
  follow-up reading.
- Explain testing program logic and command results with script checks and the
  native `test` step, including failure reporting and parallel or matrix groups.
- Explain the custom-command example as a project-defined Atmos subcommand,
  with YAML supplying its interface and the script implementing its behavior.

- Name standalone executables **custom CLI apps**, feature them first on the
  overview, and use that terminology in the guide, sidebar, example, and changelog.
  Keep **Custom Commands** for the existing YAML integration. Show the complete
  `atmos.yaml` definition and `atmos capacity` invocation directly in its section,
  and remove standalone `.star` examples from the custom-command explanation.

- Give each chapter and guide an outcome-focused second sentence using
  “Use … when you want to …”. Match cast captions to their entry points:
  executable CLI apps, YAML-defined custom commands, and lifecycle hooks.

- Add a separate interpreter announcement focused on executable custom CLI apps,
  and keep the original language announcement focused on embedded automation.
- Use `ui.info` and `ui.success` for the focused examples' human-facing output.
  Regenerate the three casts and validate their styled output with Starlark.
  Update CLI checks to expect these UI messages on stderr.

- Reconcile roadmap conflicts while updating the PR stack against `main`:
  retain the Automation landing page and typed-input description while adopting
  the shared step reference's new `/steps` routes. Update Automation guides,
  announcements, example READMEs, and YAML include links to those canonical routes.

## Validation

- The “Why Starlark?” update passed the production website build and all nine
  navigation tests. Browser checks confirmed the new overview and reference
  sections and the interpreter announcement's reference link rendered correctly.
  Reviewed a screenshot of the language rationale; no browser errors were reported.

- The initial stack-sync website build caught stale `/workflows/steps` links
  introduced by the stack. Updated their destinations to the canonical `/steps` routes;
  the rebuilt production website passed with link validation enabled.
- Stack-sync conflict checks retained all six Automation guides, the new Steps
  navigation, and the complete script reference. All nine website navigation
  tests passed; vendor-lock file hashes remained unchanged.

- The terminology and interpreter-post update passed the production website build.
  Focused example CLI tests and test-case schema validation passed (47.848 seconds).
  Browser inspection confirmed separate Custom CLI Apps and Custom Commands
  navigation, the inline YAML definition, and the new interpreter announcement.
  Styled cast output was also rendered for visual inspection.

- Re-ran all three focused cast validators through Atmos's Starlark interpreter;
  all passed. The custom-command recording shows `cat atmos.yaml` and
  `atmos capacity --replicas 3`; the CLI-app recording runs
  `./summarize.star services.json`; the hook recording shows accepted and rejected
  Terraform plans.

- The final production website build passed with all six Automation guides.
  Browser checks verified each guide's title, documentation sidebar, Automation
  breadcrumb, and absence of page errors. The homepage card navigated to
  `/automation`; the overview includes Testing on desktop and mobile, with no
  horizontal overflow at 390 pixels. Screenshots are saved in `.context/`.

- Ran the testing guide's exact YAML: the normal suite passed both checks; changing
  the worker count from 12 to 11 failed one check, allowed the independent check
  to pass, and exited nonzero. A real custom-command rejection check passed after
  verifying both its nonzero exit code and expected diagnostic.

- Executed the new language example and workflow guide's YAML against the rebuilt
  binary; both printed `Total workers: 12`. The documented workflow dry run passed.
- Rechecked the custom-command example's explicit flag, default, and help output,
  and the standalone example's output and help. The guides embed those existing
  source fixtures and casts.

- The shebang quick start and equivalent `atmos hello.star platform` invocation
  both printed `Hello, platform!`.
- `npm run build` passed with the new route, sidebar, and three cast embeds.
- Browser inspection of the production build returned HTTP 200, rendered the
  overview and examples, and reported no page errors. Desktop screenshots were
  saved in `.context/` and reviewed.
- The development server exhausted its heap, including a retry with an 8 GiB
  heap. Browser checks used the successfully built static site instead.

## Follow-ups

None.
