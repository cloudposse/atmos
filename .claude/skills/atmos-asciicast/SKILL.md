---
name: atmos-asciicast
description: Create and update Atmos repository asciicast demos for internal documentation, website publishing, and CastPlayer embeds.
---

# Atmos Asciicast Development Skill

Use this Claude skill when creating or updating committed `.cast` recordings inside the Atmos repository.

This is an internal Atmos development skill. It is intentionally separate from distributed Agent Skills. Do not symlink it to `agent-skills`, copy internal website assumptions into Agent Skills, or treat Claude skills and Agent Skills as synchronized artifacts.

## Intent

Atmos casts are product demos, regression evidence, and documentation examples at the same time. They should show users that Atmos workflows are simple to follow and that the feature being demonstrated works in a realistic project.

- Tell a small story a user can follow: inspect context, run the Atmos command, show the result.
- Prefer Atmos-native commands and workflow features over clever shell expressions.
- Keep recorded steps light. A visible step should usually be one command with one teaching purpose.
- Use hidden setup, cleanup, and fixture preparation only to make the recorded story deterministic.
- Treat each cast as evidence for a feature: include the command output that proves the behavior, then validate the committed `.cast`.

## Defaults

- Use shared cast defaults when available instead of repeating terminal settings on each cast step:

  ```yaml
  defaults:
    cast: !include cast-defaults.yaml .cast
    simulate: !include cast-defaults.yaml .simulate
  env: !include cast-defaults.yaml .env.recording
  ```

- Keep common recording settings in `cast-defaults.yaml` under `cast`, `simulate`, and `env` (`env.command` for command setup, `env.recording` for the recorded process).
- Write curated Atmos docs casts under `website/static/casts/...`; they are served from `/casts/...`.
- Use `type: cast` with `mode: steps` for deterministic command demos that need exit-code propagation.
- Use `mode: session` only when the demo must show typed input, prompts, key presses, or terminal timing.
- Keep ad hoc local recordings in the XDG cache via `--cast`; do not commit cache recordings.

## Fixture Policy

- Use deterministic Atmos demo fixtures under `demo/casts`.
- Do not reuse product examples or test fixtures just to make a docs cast easier.
- Keep demo output stable: no local absolute paths, hostnames, real account IDs, secrets, random IDs, or live timestamps.

## Workflow Patterns

- Put repeatable setup in `atmos casts setup` using `type: workdir` for fixture copies and a local `GOBIN` for the Atmos binary.
- Keep pre-recording `clean` steps simple; they remove stale `.cast` files before recording and do not need failure cleanup semantics.
- Put cleanup-after-recording steps after the `type: cast` step and use `when: always` when they restore secrets, stop services, or remove Terraform state.
- Leave validation steps success-only so validation runs only after recording and required cleanup complete successfully.
- Use `output: none` for noisy setup, reset, and cleanup commands that should not be part of the story.
- Prefer `type: cast` `mode: steps` with nested `type: shell` steps for command demos; add `type: simulate` steps only when typed prompts make a longer story easier to follow.
- Use `type: toast` for short recorded status narration instead of shell `printf` output.
- Keep large shell scripts out of recorded casts. If unavoidable, hide them in setup/cleanup and explain the user-facing result with lightweight Atmos commands in the recording.
- Use path-based custom command names for demo casts, for example `casts generate demo fixtures native-terraform plan`, and publish fixture casts under `/casts/demo/fixtures/...`.

## Authoring Checklist

1. Define the user-facing story before editing YAML: what feature is being proven, what command should the user remember, and what output proves it worked?
2. Add or update the workflow/custom command that regenerates the cast.
3. Regenerate the `.cast` into `website/static/casts`.
4. Review the cast as plain text for secrets, local paths, unstable timestamps, noisy logs, and shell complexity that distracts from Atmos.
5. Embed it with the website `CastPlayer` component when the corresponding docs page can show it usefully:

   ```mdx
   <CastPlayer src="/casts/cli/describe-component.cast" title="describe component" chrome controls scrubber />
   ```

6. Commit only `.cast` files: the website renders them client-side, so no display derivatives are needed. Commit GIF/MP4/PNG/JPEG derivatives only when a publishing target cannot consume the player.

## Static Screengrabs

Docs screengrabs (CLI help output, command output snapshots) are plain `.cast` recordings committed under `website/static/casts/screengrabs/` and rendered as a single static frame with the CastPlayer `static` prop:

```mdx
<CastPlayer title="atmos about --help" src="/casts/screengrabs/atmos-about--help.cast" static chrome />
```

Static mode shows the final terminal content of the whole recording at natural height (no playback, no controls, no viewport clipping). The bulk help screengrabs are generated by the `demo/screengrabs` Go driver from `demo-stacks.txt`; story-style demo casts (e.g. `learn/mindset`) are custom commands under `demo/casts/atmos.d/screengrabs/`.

## Static Render Formats (CLI)

`atmos cast render` supports native static outputs in addition to GIF/MP4: `--html` (inline-styled span fragment), `--ascii` (plain text, no ANSI), `--png`, and `--jpg`. These are rendered in-process from the recording's final terminal content — no `aha`, `agg`, or `ffmpeg` required. The global `--cast` flag accepts the same extensions (`--cast=out.png`), including on `--help` invocations. Use these for artifacts outside the website (GitHub embeds, image exports, machine-readable text); the website itself consumes raw casts.
