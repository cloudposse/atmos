---
name: atmos-asciicast
description: Create and update Atmos repository asciicast demos for internal documentation, website publishing, and CastPlayer embeds.
---

# Atmos Asciicast Development Skill

Use this Claude skill when creating or updating committed `.cast` recordings inside the Atmos repository.

This is an internal Atmos development skill. It is intentionally separate from distributed Agent Skills. Do not symlink it to `agent-skills`, copy internal website assumptions into Agent Skills, or treat Claude skills and Agent Skills as synchronized artifacts.

## Defaults

- Use `width: 120` and `height: 36` unless the target docs page needs a narrower recording.
- Write curated Atmos docs casts under `website/static/casts/...`; they are served from `/casts/...`.
- Use `type: cast` with `mode: steps` for deterministic command demos that need exit-code propagation.
- Use `mode: session` only when the demo must show typed input, prompts, key presses, or terminal timing.
- Keep ad hoc local recordings in the XDG cache via `--cast`; do not commit cache recordings.

## Fixture Policy

- Use deterministic Atmos demo fixtures under `demo/casts`.
- Do not reuse product examples or test fixtures just to make a docs cast easier.
- Keep demo output stable: no local absolute paths, hostnames, real account IDs, secrets, random IDs, or live timestamps.

## Authoring Checklist

1. Add or update the workflow/custom command that regenerates the cast.
2. Regenerate the `.cast` into `website/static/casts`.
3. Review the cast as plain text for secrets, local paths, unstable timestamps, and noisy logs.
4. Embed it with the website `CastPlayer` component when the corresponding docs page can show it usefully:

   ```mdx
   <CastPlayer src="/casts/cli/describe-component.cast" title="describe component" chrome controls scrubber />
   ```

5. Prefer committing only `.cast` files. Commit GIF/MP4/SVG derivatives only when a publishing target cannot consume the player.
