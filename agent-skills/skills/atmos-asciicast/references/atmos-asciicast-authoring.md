# Atmos Asciicast Authoring

## Defaults

- Use shared cast defaults when the host project supports YAML includes or reusable workflow configuration. Keep terminal settings such as `rate`, `width`, and `height` together instead of repeating them on every cast step.
- Keep simulated input defaults such as prompt text, cursor behavior, typing rate, and jitter together with the cast defaults when the project has several recordings.
- Use `type: cast` with `mode: steps` for deterministic command demos that need exit-code propagation.
- Use `mode: session` only when the demo must show typed input, prompts, key presses, or terminal timing.
- Keep ad hoc local recordings in the XDG cache via `--cast`; do not commit cache recordings.
- Store committed casts wherever the host project keeps documentation assets.

## Fixture Policy

- Prefer small deterministic fixtures owned by the documentation example.
- Do not reuse unrelated product examples or test fixtures just to make a recording easier.
- Keep demo output stable: no local absolute paths, hostnames, real account IDs, secrets, random IDs, or live timestamps.

## Authoring Checklist

1. Identify the exact command sequence and fixture data that the audience should learn from.
2. Add or update the workflow/custom command that regenerates the cast when the host project supports that pattern.
3. Regenerate the `.cast` into the project-approved documentation asset location.
4. Review the cast as plain text for secrets, local paths, unstable timestamps, noisy logs, and project-specific assumptions.
5. Embed or link the cast using the host project's documentation conventions.
6. Prefer committing only `.cast` files unless the host project explicitly documents additional generated artifacts.

Prefer first-class workflow steps over shell output for recorded narration. For example, use `type: toast` for short status messages instead of `printf` in a recorded shell step.

## Boundaries

- Do not include internal Atmos development workflows in this Agent Skill.
- Do not reference Claude skills or assume a sync relationship with them.
- Do not hard-code Cloud Posse website paths or internal demo fixture locations in community guidance.
