# Landing-page demos (VHS)

Animated terminal recordings for the homepage sections. Each `*.tape` runs a real,
on-topic Atmos command — most against a local cloud **emulator**, so the demos show
actual provisioning with no cloud credentials. The renders are embedded by
[`website/src/components/landing/DemoVideo`](../../website/src/components/landing/DemoVideo).

## The recorder is an Atmos custom command

Recording is wrapped as `atmos demo …` (defined in [`atmos.yaml`](./atmos.yaml)), so the
exact same command runs on a laptop and in CI — the page's own "Local = CI" thesis:

```shell
cd demo/landing
atmos demo check hero        # FAST dry-run → readable .ascii, short timeout (do this first!)
atmos demo record hero       # full render of one tape → website/static/img/demos/
atmos demo record all        # render every tape
atmos demo publish           # aws s3 sync renders to the docs S3/CDN origin
atmos demo clean             # remove local renders + stop stray emulator containers
```

**Always `atmos demo check <name>` first.** It renders the tape to inspectable text
(`website/static/img/demos/<name>.ascii`) with a 90s timeout and prints the final frame — so a
broken command or a stuck `Wait` shows up in seconds, not after a multi-minute video render. Only
`record` once the dry-run looks right. (`make build` first — see prerequisites; the recorder puts
`./build/atmos` on PATH so the demos use this branch's `atmos`, not an older one.)

CI runs the same entrypoint: [`.github/workflows/landing-demos.yaml`](../../.github/workflows/landing-demos.yaml).

## The renders are never committed

`website/static/img/demos/` is **gitignored**. The binaries (`*.webm`, `*.mp4`, poster
`*.png`) are synced to S3/CDN by `atmos demo publish`; the site references the remote URL
via `customFields.demosBaseUrl` (env `ATMOS_DEMOS_BASE_URL`). When that base is empty (local
dev), `DemoVideo` falls back to the gitignored local copies so you can preview before publishing.

## Prerequisites

- The recording toolchain — [`vhs`](https://github.com/charmbracelet/vhs) and its `ffmpeg`
  runtime dependency — is **auto-installed** by `atmos demo record`/`check` via Atmos
  toolchain (declared under `toolchain:` in [`atmos.yaml`](./atmos.yaml), pinned in each
  command's `dependencies.tools`). FFmpeg publishes source only, so the aqua-managed
  `ffmpeg` alias uses the third-party `Tyrrrz/FFmpegBin` prebuilt distribution.
- `ttyd` is required by VHS at runtime, but it is intentionally **not** a command dependency
  because its aqua package has no macOS binary. Install it once with `brew install ttyd` on
  macOS; CI installs it from the same Atmos toolchain on Linux.
- The `FiraCode Nerd Font` is expected for the rendered glyphs.
- A container runtime (Docker or Podman) for the emulator-backed tapes — this is the one
  thing that can't be auto-installed (it's a system daemon, not a single binary).
- The k8s example installs `helmfile`, `helm`, and `kubectl` through Atmos toolchain
  dependencies; `emulators.tape` brings up the AWS/GCP/Azure (Floci) and k3s emulators.
- The AWS CLI for `atmos demo publish`.
- **A locally built atmos** (`make build`, on `PATH`): the emulator commands these tapes use
  are newer than the latest published release.

## How a tape is structured

Each tape sources [`defaults.tape`](./defaults.tape) (shared look + the wait behavior), then:

1. **Hidden setup** — sets a stable green `❯ ` prompt, `cd`s into the example, and (usually)
   starts the emulator off camera.
2. **On camera** — types the topical command(s). A bare `Wait` blocks until the command
   finishes, anchored on the `❯ ` prompt (`WaitPattern` in `defaults.tape`). This is the VHS
   "wait for output" capability that makes recording *real* (variable-duration) commands
   possible — no brittle `Sleep` guesses.
3. **Hidden teardown** — destroys resources and stops the emulator.

If a command's output or timing changes and a `Wait` hangs, adjust `WaitTimeout`/`WaitPattern`
in `defaults.tape`.

> [!IMPORTANT]
> **Do not use `Wait+Screen /regex/` for commands whose output scrolls** (e.g.
> `terraform plan`/`apply`/`deploy`). VHS bug
> [charmbracelet/vhs#657](https://github.com/charmbracelet/vhs/issues/657) /
> [#659](https://github.com/charmbracelet/vhs/issues/659): once the terminal scrolls,
> `Wait+Screen` (and `.ascii`/`.txt` capture) search the **top of the scrollback** instead
> of the visible viewport, so the match times out and the recording aborts — even though the
> **video renders the scrolled output correctly** (the bug is only in VHS's text-capture
> path, not the recording). Use a bare `Wait /❯$/` (wait for the prompt to return) for those
> commands. `Wait+Screen` is fine only when the matched text stays within one screen (no
> scroll). This is also why long terraform demos do **not** need `--mask`/`ATMOS_MASK`
> tricks: masking doesn't affect the video; the only requirement is the right `Wait`.

| Tape | Section | Example | Emulator |
|------|---------|---------|----------|
| `hero.tape` | Hero | `examples/demo-stacks` | none |
| `how-it-works.tape` | How It Works | temp copy of `demo/landing/fixtures/how-it-works` | none |
| `terraform.tape` | Terraform & OpenTofu | `demo/landing/fixtures/terraform` | none |
| `kubernetes.tape` | Kubernetes & Helm | `examples/emulator-k8s` | k3s |
| `emulators.tape` | Containers & Emulators | `demo/landing/fixtures/emulators` | aws/gcp/azure (Floci) + k3s |
| `local-ci.tape` | Local = CI | temp repo from `examples/demo-stacks` | none |
| `secrets.tape` | Secrets & Stores | temp copy of `demo/landing/fixtures/secrets` | none |
| `dx.tape` | Developer experience | `demo/landing/fixtures/dx` | aws |
| `extensibility.tape` | Extensible by design | temp copy of `demo/landing/fixtures/extensibility` | none |
