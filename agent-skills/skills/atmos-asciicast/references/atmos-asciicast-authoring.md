# Atmos Asciicast Authoring

## Defaults

- Use shared cast defaults when the host project supports YAML includes or reusable workflow configuration. Keep terminal settings such as `rate`, `width`, and `height` together instead of repeating them on every cast step.
- Keep simulated input defaults such as prompt text, cursor behavior, typing rate, and jitter together with the cast defaults when the project has several recordings.
- Use `type: cast` with `mode: steps` for deterministic command demos that need exit-code propagation.
- Use `mode: session` only when the demo must show typed input, prompts, key presses, or terminal timing.
- When a `mode: steps` demo logically enters a generated or nested directory, add a synthetic `type: simulate` `cd <directory>` before the first command shown there. Keep every real child step's `working_directory` explicit: simulated input does not change subprocess state.
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
4. Run its Starlark validator, then review the cast as plain text for secrets, local paths, unstable timestamps, noisy logs, and project-specific assumptions.
5. Embed or link the cast using the host project's documentation conventions.
6. Prefer committing only `.cast` files unless the host project explicitly documents additional generated artifacts.

Prefer first-class workflow steps over shell output for recorded narration. For example, use `type: toast` for short status messages instead of `printf` in a recorded shell step.

## Validate casts with Starlark

Prefer an embedded Starlark step for new cast validators and when revising existing
Python validators. Use `fs.read_file`, `json.decode`, `regex`, and `fail` to validate
the actual recording without adding a Python runtime dependency. Keep reusable
assertions in a `.star` module and load it from per-demo validators.

For example, save this as `scripts/validate-cast.star`, with the step's working
directory set to the project root:

```starlark
def validate(path):
    lines = fs.read_file(path).splitlines()
    if not lines:
        fail("empty cast: " + path)
    header = json.decode(lines[0])
    if header.get("version") not in [2, 3]:
        fail("unsupported cast version")
    events = [json.decode(line) for line in lines[1:] if line.strip()]
    text = "".join([event[2] for event in events if event[1] in ["o", "e"]])
    plain = regex.replace(r"\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)", "", text)
    if "expected result" not in plain:
        fail("cast is missing the expected result")
    ui.success("Cast validated")

validate("recordings/demo.cast")
```

Replace the example's
path and expected result with the demo's actual evidence. Attach it after the cast
and required cleanup, on the normal success path:

```yaml
- type: script
  name: validate
  interpreter: starlark
  script: !include scripts/validate-cast.star
```

Check forbidden secrets and machine-specific paths in the reconstructed text,
including values split across events. Check ANSI styling against raw text and
content against stripped text. For demos that intentionally fail, assert the
expected failure rather than rejecting all error output. Run the validator after
regeneration and still review playback for pacing and layout.

## Boundaries

- Do not include internal Atmos development workflows in this Agent Skill.
- Do not reference Claude skills or assume a sync relationship with them.
- Do not hard-code Cloud Posse website paths or internal demo fixture locations in community guidance.
