# Atmos GitHub Runtime action

Exposes the GitHub Actions **runtime credentials** (`ACTIONS_RUNTIME_TOKEN`,
`ACTIONS_RESULTS_URL`, …) to your `run:` steps so the Atmos backends that talk
to the runner's cache and artifacts services can reach them:

- **Cache** — `atmos ci cache restore` / `save` (GitHub Actions cache service).
- **Planfile storage** — `atmos terraform plan --ci` / `atmos terraform deploy`
  / `atmos terraform planfile upload`|`download` when the `github/artifacts`
  store is selected (GitHub Actions Artifacts service).

Both read the same two env vars, so this single action serves both.

## Why this exists

GitHub injects `ACTIONS_RUNTIME_TOKEN` / `ACTIONS_RESULTS_URL` into the process
environment of **action steps** (`uses:`) but **not** into `run:` shell steps —
a deliberate least-privilege decision (those credentials can read/write the
cache and artifacts, so they're kept out of arbitrary shell scripts). A
JavaScript action *does* receive them, so this action re-exposes them.

> If you don't need Atmos to own restore/save, prefer the native
> [`actions/cache`](../cache/README.md) integration (via `atmos ci cache paths`)
> — it needs **no** runtime token at all and is the most secure option.

## Modes

| `mode` | What it does | Security |
| --- | --- | --- |
| `output` (default) | Emits masked **step outputs** (`runtime-token`, `results-url`, `cache-url`, `runtime-url`). You thread them only into the steps that need them via `env:`. | **Least privilege** — only the steps you wire get the token. |
| `env` | Exports every `ACTIONS_*` var to `$GITHUB_ENV`. | Convenient, but the token is **ambient** to every later `run:` step in the job (largest blast radius). |

The runtime token is always masked with `::add-mask::` regardless of mode.

## Usage

### `mode: output` (recommended)

```yaml
- uses: cloudposse/atmos/actions/github-runtime@v1   # pin to a release or SHA
  id: ghr
- run: atmos ci cache restore
  env:
    ACTIONS_RUNTIME_TOKEN: ${{ steps.ghr.outputs.runtime-token }}
    ACTIONS_RESULTS_URL:   ${{ steps.ghr.outputs.results-url }}
- run: atmos toolchain install --default helm/helm@v3.16.0
- if: always()
  run: atmos ci cache save
  env:
    ACTIONS_RUNTIME_TOKEN: ${{ steps.ghr.outputs.runtime-token }}
    ACTIONS_RESULTS_URL:   ${{ steps.ghr.outputs.results-url }}
```

### `mode: env`

```yaml
- uses: cloudposse/atmos/actions/github-runtime@v1
  with:
    mode: env
- run: atmos ci cache restore
- run: atmos toolchain install --default helm/helm@v3.16.0
- if: always()
  run: atmos ci cache save
```

### Planfile storage (`github/artifacts`)

The same credentials let the [`github/artifacts` planfile store](https://atmos.tools/ci/planfile-storage)
upload/download planfiles from a `run:` step. With `mode: env` no per-step wiring is needed —
`atmos terraform` reads the credentials from the environment automatically:

```yaml
- uses: cloudposse/atmos/actions/github-runtime@v1   # pin to a release or SHA
  with:
    mode: env
- run: atmos terraform plan mycomponent -s prod --ci    # uploads to github/artifacts
- run: atmos terraform deploy mycomponent -s prod --ci  # downloads & verifies the planfile
```

Prefer `mode: output` (the default) when you want to scope the credentials to only these steps via
explicit `env:`, exactly as in the cache examples above.

## Versioning

This action ships **inside the Atmos repository**, so the ref is an Atmos
release: pin to `@v1` (moving major tag), `@vX.Y.Z` (a specific release), or a
commit SHA for full reproducibility.

## Implementation

[`index.js`](./index.js) is a dependency-free `node24` action. It iterates
`process.env`, masks any value whose name contains `TOKEN`, and either writes
the four named outputs to `$GITHUB_OUTPUT` or appends every `ACTIONS_*` var to
`$GITHUB_ENV` (heredoc for multiline values). Audit it before pinning.
