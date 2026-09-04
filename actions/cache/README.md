# Atmos Cache action

A one-line cache for Atmos-managed directories (toolchain installs and anything
under the Atmos cache root). Atmos supplies **what** to cache — the key, paths,
and restore-keys from your `ci.cache` configuration — and the native
[`actions/cache`](https://github.com/actions/cache) does the storage.

This is the **recommended** way to cache Atmos in GitHub Actions: it exposes
**no** runtime token to your job (Atmos only emits non-secret key/paths), so it's
the most secure option, with the least boilerplate.

## Usage

```yaml
- uses: cloudposse/atmos/actions/cache@v1     # pin to a release or SHA
- run: |
    atmos toolchain install --default helm/helm@v3.16.0
    atmos toolchain install --default helmfile/helmfile@v1.1.7
    atmos toolchain env --format=github
```

That's it. The action runs `atmos ci cache paths --format=github` to resolve the
key/paths from `ci.cache`, then calls `actions/cache` with them.

> Requires the `atmos` binary on `PATH` — install it (e.g. via
> `cloudposse/github-action-setup-atmos`) before this step.

### Configuration

Define the cache key/paths once in `atmos.yaml`:

```yaml
ci:
  cache:
    enabled: true
    key: 'atmos-toolchain-{{.OS}}-{{.Arch}}-v1'
    restore_keys:
      - 'atmos-toolchain-{{.OS}}-{{.Arch}}-'
```

### Inputs

| Input | Default | Description |
| --- | --- | --- |
| `mode` | `'restore-and-save'` | `restore-and-save`: restore and save in the post step (`actions/cache`). `restore-only`: restore but never save (`actions/cache/restore`, no post step). |

#### Many parallel consumers, one writer

If several jobs (for example, a matrix of test shards) restore the same key,
let exactly one job save it and put the rest on `mode: restore-only`. Otherwise every
shard that misses tries to save the same key at the end of the job: all but
one fail with `Unable to reserve cache with key ..., another job may be
creating this cache`, and each still pays the tar + zstd + upload cost first.

```yaml
# The single writer (saves on a miss):
- uses: cloudposse/atmos/actions/cache@v1

# Every parallel consumer (restores only, no post step):
- uses: cloudposse/atmos/actions/cache@v1
  with:
    mode: restore-only
```

### Outputs

| Output | Description |
| --- | --- |
| `cache-hit` | `true` when `actions/cache` (or `actions/cache/restore`) found an exact key match. |
| `key` | The resolved cache key. |

## How it compares

| Option | Token exposed? | Boilerplate |
| --- | --- | --- |
| **`actions/cache` (this action)** | **No** | One `uses:` |
| `atmos ci cache paths` + `actions/cache` (manual) | No | Three steps |
| [`actions/github-runtime`](../github-runtime/README.md) + `atmos ci cache restore/save` | Yes (masked) | Per-step `env:` or ambient |

If you need Atmos to *own* restore/save (rather than `actions/cache`), use the
[`github-runtime`](../github-runtime/README.md) action instead.

## Versioning

This action ships inside the Atmos repository, so the ref is an Atmos release:
pin to `@v1` (moving major tag), `@vX.Y.Z`, or a commit SHA. It internally pins
`actions/cache` and `actions/cache/restore` to the same SHA (`v5.0.5`).
