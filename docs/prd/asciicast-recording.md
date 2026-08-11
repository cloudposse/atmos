# PRD: Asciicast Recording and Cast Artifacts

**Status**: Proposed
**Created**: 2026-06-30
**Owner**: Engineering Team

## Summary

Add Atmos-native terminal recording using asciicast-compatible JSON output. Cast recording captures Atmos I/O once, stores the recording as a reusable artifact, and enables later playback or rendering to formats such as SVG, GIF, and MP4.

The feature has two surfaces:

1. A global `--cast` flag that records any Atmos command invocation.
2. A shared `type: cast` step for workflows and step-based custom commands.

Recording is opt-in. If cast recording is not explicitly enabled by CLI flag or configuration, Atmos does not write cast artifacts.

## Goals

- Record command I/O through the Atmos I/O layer so masking, stdout/stderr separation, and UI/data behavior remain consistent.
- Make generated cast paths collision-resistant without requiring users to configure filename templates.
- Keep the global CLI surface small: one global `--cast` flag.
- Store casts in the Atmos XDG cache by default, with a catalog-friendly layout.
- Support workflows and custom commands through the shared step system.
- Preserve real step exit-code semantics for normal step recording.
- Provide a built-in cast player so recordings can be replayed without immediately rendering to video.
- Establish a curated command demo suite, starting with Terraform and vendor commands.
- Provide a reusable website component for embedding published casts on documentation pages.
- Provide an agent skill for creating consistent Atmos cast demos.
- Use Atmos workflows or custom commands as the automation surface for regenerating committed demo casts.
- Make committed cast demos part of the definition of done for new Atmos commands after this feature ships.

## Non-Goals

- V1 does not add filename templating.
- V1 does not add `--cast-overwrite` or other cast-specific global flags.
- V1 does not require users to configure a catalog format before recording.
- V1 does not add a general-purpose `expect` assertion DSL for normal workflow/custom-command steps.
- V1 does include a narrow `wait` action inside `mode: session` casts so typed-shell automation can synchronize on PTY output.
- Session waits support both literal text and regular expressions, matching the core VHS expectation behavior.
- V1 does not guarantee per-command exit codes inside a single typed interactive shell session.
- V1 does not require demo recordings for every Atmos command before shipping.

## Public Interface

### Global Flag

`--cast` is a root/global flag with an optional value:

```sh
atmos --cast terraform plan vpc -s plat-ue2-dev
atmos --cast=artifacts/demo.cast terraform plan vpc -s plat-ue2-dev
```

Behavior:

- `--cast` enables recording and writes to an auto-generated unique path under `cast.recording.base_path`.
- `--cast=<path>` enables recording and writes to the explicit cast file path.
- Bare `--cast` must not consume the next command token as a path. The explicit path form uses `--cast=<path>`.
- Generated paths never overwrite existing files.
- Explicit paths fail if the target already exists.
- Atmos emits the final "Cast recorded" notice after the recorder is closed so the notice is not included in the cast.

### Configuration

Cast configuration is a top-level `cast:` namespace. It is not nested under `settings:` because it belongs to the cast command family and cast artifact behavior.

```yaml
cast:
  recording:
    enabled: false
    base_path: ""
    input: false
    width: 120
    height: 36
```

Field semantics:

- `enabled`: enables recording without passing `--cast`.
- `base_path`: root folder for generated cast files. Empty means use the Atmos XDG cache default.
- `input`: captures input events only when explicitly enabled.
- `width` and `height`: terminal dimensions to record when they cannot be detected or when a deterministic size is needed.

Use `base_path` for the generated cast root because Atmos already uses `base_path` for configurable roots such as `components.*.base_path`, `stacks.base_path`, `workflows.base_path`, `schemas.*.base_path`, `profiles.base_path`, and `vendor.base_path`.

Use `path` only for explicit concrete files, such as `--cast=<path>` or step output file names.

The default generated cast root is an XDG cache location, not a project-local `.atmos` directory. The intended default is:

```text
<atmos-xdg-cache>/casts
```

For a typical local environment this resolves to a path like:

```text
~/.cache/atmos/casts
```

### Generated Paths

When no explicit file path is supplied, Atmos writes casts under:

```text
<base_path>/YYYY/MM/DD/HHMMSS-command-slug-runid.cast
```

Example:

```text
~/.cache/atmos/casts/2026/06/30/142233-terraform-plan-vpc-8f3c2a.cast
```

Path components:

- `YYYY/MM/DD`: UTC recording date.
- `HHMMSS`: UTC recording start time.
- `command-slug`: sanitized command words, excluding global Atmos boilerplate.
- `runid`: short random or run-scoped identifier to avoid collisions for repeated commands in the same second.

This layout is intentionally opinionated in v1. Users can change the root with `cast.recording.base_path`, but not the filename structure.

### Completion Notice

At the end of a recorded invocation, Atmos prints a user-facing notice outside the cast:

```text
Cast recorded: ~/.cache/atmos/casts/2026/06/30/142233-terraform-plan-vpc-8f3c2a.cast
```

Rules:

- Close and detach the recorder before emitting the notice.
- Write the notice to the UI channel, not stdout.
- If render artifacts are produced later, emit those artifact paths outside the recording too.

## Cast Step

`type: cast` is implemented in the shared step registry so it works anywhere Atmos steps work, including workflows and step-based custom commands.

```yaml
steps:
  - type: cast
    name: demo-atmos-deploy
    mode: session
    shell: bash
    width: 120
    height: 36
    write_rate: 40ms
    output:
      cast: artifacts/demo.cast
      gif: artifacts/demo.gif
      mp4: artifacts/demo.mp4
    steps:
      - type: write
        text: atmos describe component api -s plat-ue2-dev

      - type: key
        key: enter

      - type: wait
        text: "Component: api"

      - type: pause
        duration: 1s

      - type: write
        text: atmos deploy api -s plat-ue2-dev
        rate: 20ms

      - type: key
        key: enter

      - type: wait
        text: "Proceed?"

      - type: write
        text: "y"

      - type: key
        key: enter

      - type: wait
        text: "Deployment successful"
```

Modes:

- `mode: steps`: records nested normal Atmos steps. Each nested step keeps normal exit-code and failure behavior.
- `mode: session`: starts one PTY shell and drives it with terminal actions such as `write`, `key`, `wait`, and `pause`.

Session waits:

- `type: wait` is valid only inside `mode: session` cast steps.
- `wait.text` waits until the active PTY output contains the literal text.
- `wait.regex` waits until the active PTY output matches a regex.
- Exactly one of `text` or `regex` must be set.
- Regex syntax follows Go's `regexp` package.
- Invalid regex patterns fail validation before the session starts.
- `wait.timeout` overrides the cast-level wait timeout for that wait action.
- A wait timeout fails the cast step.
- `wait` is a synchronization action, not a post-run assertion. It does not validate exit codes.

Example:

```yaml
- type: wait
  text: "Proceed?"
  timeout: 30s

- type: wait
  regex: "Deployment (successful|complete)"
  timeout: 2m
```

Normal step recording does not require waits because each nested command runs to completion and returns an exit code through the normal step executor.

Write rate:

- `write_rate` sets the cast-level default delay between text writes.
- `rate` on a `write` step overrides the default for that action.
- `rate: 0` writes the whole text immediately.
- The default `write_rate` is implementation-defined but should be human-readable for demos.

Key repeats:

- `repeat` on a `key` step sends the same key more than once. Default is `1`.
- `interval` on a `key` step sets the delay between repeated key events. Default is the cast-level `key_interval`.
- `key_interval` sets the cast-level default delay between repeated key events.
- A `key` step without `repeat` sends exactly one key event.

Example:

```yaml
- type: write
  text: atmos describe component api -s plat-ue2-dev
  rate: 20ms

- type: key
  key: down
  repeat: 3
  interval: 50ms
```

Output fields:

- `output.cast`: explicit cast file path. Fail if it already exists.
- `output.gif`, `output.svg`, `output.mp4`: optional render outputs derived from the cast.
- If `output.cast` is omitted, generate a unique cast path under `cast.recording.base_path`.

## Architecture

### I/O Tap

Cast recording taps the Atmos I/O layer rather than individual commands. This is required so recording works for built-in commands, workflows, custom commands, hooks, and component commands.

Implementation requirements:

- Record after masking, never before masking.
- Preserve stdout/stderr semantics internally even if the final asciicast display stream is terminal-like.
- Record input only when `cast.recording.input` or an equivalent explicit option is enabled.
- Record resize events when terminal dimensions change.
- Do not record the post-run "Cast recorded" notice.

### Artifact Storage

The `.cast` file is the canonical artifact. Rendered SVG/GIF/MP4 files are derived artifacts.

Generated cast metadata should be catalog-friendly even if the v1 catalog is minimal. At minimum, record enough metadata in a sidecar or index to list recent casts later:

- ID
- Cast path
- Started time
- Completed time
- Command
- Working directory
- Exit code
- Git branch and SHA when available

## Player, Render, and Playback

Add a cast command family for operating on recordings:

```sh
atmos cast render input.cast --svg output.svg
atmos cast render input.cast --gif output.gif
atmos cast render input.cast --mp4 output.mp4
atmos cast play input.cast
```

### Player

Atmos should include a terminal cast player for `.cast` files.

Player requirements:

- `atmos cast play <file.cast>` replays the recording in the terminal.
- Playback honors recorded timing by default.
- Playback should expose a seek/scrub capability where the runtime supports it.
- Playback supports a speed multiplier in command-specific flags, not global flags.
- Playback can optionally skip idle time so long recordings are easy to review.
- Playback does not require SVG/GIF/MP4 render dependencies.
- Playback should read asciicast-compatible v2 files produced by Atmos and by standard asciinema tooling where practical.

The player is useful for local demos, support handoff, and validating generated recordings before upload or render.

### Render

Renderer behavior:

- Prefer external tools in v1 rather than implementing terminal rendering from scratch.
- Use `agg` or equivalent asciicast renderer for GIF where available.
- Use FFmpeg for MP4 generation where needed.
- Fail with an actionable missing-tool error when a requested renderer is unavailable.

## Command Demo Suite

Cast recording should support a curated demo suite that can generate reusable recordings for important Atmos commands.

Demo recordings are different from ad hoc user recordings:

- Ad hoc recordings created with `--cast` default to the Atmos XDG cache.
- Curated demo recordings are source artifacts and should be committed to the repository as plain-text `.cast` files.
- Rendered GIF/MP4 files are derived artifacts and should not be required for source control unless needed for docs publishing.

Committed demo casts should live under the website static assets tree so they are published with the documentation site:

```text
website/static/casts/
```

Those files are then addressable on the published site under:

```text
/casts/
```

The long-term goal is broad command coverage. V1 should start with a small set of high-value committed casts:

- Terraform component lifecycle:
  - `atmos terraform plan <component> -s <stack>`
  - `atmos terraform deploy <component> -s <stack>`
  - `atmos terraform output <component> -s <stack>`
- Vendor workflows:
  - `atmos vendor pull`
  - `atmos vendor diff`
- Basic introspection:
  - `atmos describe component <component> -s <stack>`
  - `atmos describe stacks`

Demo requirements:

- Demos should be expressed as `type: cast` definitions where possible so they are reproducible.
- Demos should run against dedicated cast demo fixtures, not real user infrastructure.
- Cast demo fixtures should be separate from product examples and test fixtures so docs demos do not constrain example design or test maintenance.
- Demos should use explicit output paths under `website/static/casts/` so they are stable, commit-friendly, and published on the website.
- Demo cast contents must be deterministic enough for review. Use fixture data, stable terminal dimensions, stable command output, and masking/sanitization where needed.
- Demos should prefer `mode: steps` when command success and exit-code fidelity matter.
- Demos should use `mode: session` only when showing interactive terminal behavior is important.
- Demos should be suitable for later upload or rendering to SVG/GIF/MP4.
- PRs that update demo behavior should include the updated `.cast` files when the recording output intentionally changes.

## Website Embeds

The website should provide a reusable component for embedding committed cast recordings on any docs page.

Component requirements:

- The component loads published `.cast` files from `website/static/casts/`, addressed as `/casts/...` at runtime.
- The component can render an interactive/player view when JavaScript is available.
- The component should support a static fallback or link for non-JavaScript contexts.
- The component should accept playback controls such as autoplay, loop, speed, idle skipping, and scrubbing as component props.
- The component should support a scrubber/timeline control so viewers can move backward and forward through a recording.
- The component should support optional terminal chrome so the recording appears inside a terminal window frame.
- Terminal chrome should be configurable, including title text and whether the frame is shown.
- The component should be usable from MDX pages with a small, stable API.

Example MDX usage:

```mdx
<CastPlayer
  src="/casts/terraform/plan.cast"
  title="atmos terraform plan"
  chrome
  controls
  scrubber
/>
```

Scrubber behavior:

- The scrubber maps elapsed recording time to terminal state.
- Seeking backward reconstructs the terminal state at the target time; it must not replay visibly from the beginning.
- The player may build an in-memory checkpoint/index from the asciicast event stream to make seeking efficient.
- The scrubber should remain optional so lightweight embeds can hide it.
- The CLI player may support seek through keyboard controls or flags, but the website component is the primary v1 scrubber surface.

Terminal chrome behavior:

- `chrome` wraps the player in a terminal-like frame.
- `title` appears in the frame header when `chrome` is enabled.
- The chrome is presentation only; it must not be baked into the `.cast` recording.
- Pages can disable chrome when embedding casts in tighter layouts.

## Command Documentation Policy

After cast recording and website embeds ship, every new Atmos command should include a committed cast demonstration as part of its definition of done.

Policy requirements:

- New commands must add at least one `.cast` file under `website/static/casts/`.
- The cast should demonstrate the command's primary successful workflow using dedicated cast demo fixtures, not real user infrastructure, product examples, or test fixtures.
- The corresponding command documentation should embed the cast with the website `CastPlayer` component whenever practical.
- The cast should be reviewed like source code because it is committed plain-text output.
- If a new command cannot have a useful cast demo, the PR should explain why in the command documentation or PR notes.
- Existing commands should be backfilled incrementally, starting with Terraform, vendor, and high-traffic describe/list commands.

Example command documentation embed:

```mdx
<CastPlayer
  src="/casts/cli/terraform-plan.cast"
  title="atmos terraform plan"
  chrome
  controls
  scrubber
/>
```

## Cast Authoring Skill

Provide an agent skill for creating and updating Atmos cast demos consistently.

The skill should be used whenever an agent is asked to create, refresh, review, or embed an Atmos cast recording.

Skill requirements:

- Standardize cast dimensions, write/key timing, wait usage, terminal chrome expectations, and website embed conventions.
- Prefer dedicated deterministic cast demo fixtures over real infrastructure, product examples, or test fixtures.
- Use `mode: steps` for demos where exit-code fidelity matters.
- Use `mode: session` only when the demo needs to show interactive typing, prompts, or terminal behavior.
- Require `wait.text` or `wait.regex` after session actions that need synchronization.
- Write committed demo casts to `website/static/casts/` with stable, command-oriented paths.
- Keep ad hoc recordings in the XDG cache unless the task explicitly asks for a committed website demo.
- Check recorded output for secrets, machine-specific paths, timestamps, and other unstable values before committing.
- Update the corresponding command docs with a `CastPlayer` embed when practical.
- Regenerate derived SVG/GIF/MP4 only when docs publishing needs those artifacts.

The skill should be a short operational checklist with a few canonical examples, not a duplicate of this PRD.

## Cast Regeneration Automation

Committed demo casts should be reproducible through Atmos itself.

Automation requirements:

- Provide Atmos workflows or custom commands that regenerate committed casts.
- Prefer workflows for multi-demo orchestration and custom commands for ergonomic command-specific refreshes.
- Regeneration commands should write explicit outputs under `website/static/casts/`.
- Regeneration should use dedicated cast demo fixtures and deterministic terminal dimensions.
- Regeneration should fail when expected fixture data is missing rather than producing partial casts.
- Regeneration should be safe to run locally and in CI without touching real infrastructure.
- CI may verify committed casts are current by running the regeneration workflow and checking for diffs.

Fixture separation requirements:

- Cast demo fixtures should live in a clearly named location separate from `examples/` and `tests/fixtures/`.
- Product examples remain optimized for users learning Atmos.
- Test fixtures remain optimized for automated test coverage.
- Cast demo fixtures remain optimized for stable, concise, visually useful website recordings.
- Changes to product examples or tests should not unintentionally require rerecording website casts.
- Changes to website casts should not require reshaping examples or tests unless the underlying command behavior changed.

Example shape:

```yaml
workflows:
  generate-casts:
    description: Regenerate website cast demos
    steps:
      - type: cast
        name: terraform-plan
        mode: steps
        width: 120
        height: 36
        output:
          cast: website/static/casts/cli/terraform-plan.cast
        steps:
          - type: atmos
            command: terraform plan mock -s plat-ue2-dev
```

## Test Plan

- Unit test generated cast paths: date hierarchy, command slug, run ID, and no collisions.
- Unit test `--cast` parsing so bare `--cast` does not consume the command name as a path.
- Unit test explicit path collision failure.
- Unit test config loading for `cast.recording.base_path`.
- Unit test that the completion notice is emitted after recorder shutdown and is not present in the cast file.
- Unit test masking order so recorded bytes contain redacted values only.
- Integration test global recording for a built-in command, a workflow, and a step-based custom command.
- Integration test `type: cast` in both workflow and custom command definitions.
- Renderer tests should use fake executable runners by default; real `agg`/FFmpeg tests should run only when those tools are present.

## Open Questions

- Exact catalog storage format: one append-only index, one sidecar per cast, or both.
- Whether `atmos cast list` ships with v1 or follows once enough metadata exists.
- Whether `cast.recording.enabled: true` should require `base_path` to be writable during config load or only when a command begins recording.
