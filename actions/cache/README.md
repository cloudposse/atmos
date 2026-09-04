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

### Outputs

| Output | Description |
| --- | --- |
| `cache-hit` | `true` when `actions/cache` found an exact key match. |
| `key` | The resolved cache key. |

## How it compares

| Option | Token exposed? | Boilerplate |
| --- | --- | --- |
| **`actions/cache` (this action)** | **No** | One `uses:` |
| `atmos ci cache paths` + `actions/cache` (manual) | No | Three steps |
| [`actions/github-runtime`](../github-runtime/README.md) + `atmos ci cache restore/save` | Yes (masked) | Per-step `env:` or ambient |

If you need Atmos to *own* restore/save (rather than `actions/cache`), use the
[`github-runtime`](../github-runtime/README.md) action instead.

## Windows runners

Two things matter for cache speed on Windows runners, and the action handles both.

**Which tar.** The action puts Git for Windows' GNU tar first on `PATH` before caching (when it is
installed, as on GitHub-hosted runners), because `actions/cache` otherwise falls back to Windows'
own bsdtar, whose code path decompresses the whole archive to disk before extracting it. On a
1.7 GB archive that is the difference between a 264 s and a 353 s restore. Automatic, idempotent,
and a no-op on Linux or macOS.

**Which disk.** On GitHub's `windows-latest` runners the work disk (D:, where `runner.temp` and
the workspace live) is a separate disk with roughly nine times the small-file write rate of C:,
where Atmos's default cache root (`%LOCALAPPDATA%\cache\atmos`) sits. That matters when the
cached tree is many small files (provider mirrors, plugin caches); for a few large binaries it
barely registers. Opt in with `cache-home`, which exports `ATMOS_XDG_CACHE_HOME` for the rest of
the job so toolchain installs, the cache save and the cached paths all use the same root:

```yaml
- uses: cloudposse/atmos/actions/cache@main
  with:
    cache-home: ${{ runner.temp }}/atmos
```

Two caveats. The cached paths change, so the cache version changes: the first run after setting
it is a cold start. And the root moves for the whole job, so don't set it in a job whose tests or
tooling assert Atmos's default cache locations.

## Versioning

This action ships inside the Atmos repository, so the ref is an Atmos release:
pin to `@v1` (moving major tag), `@vX.Y.Z`, or a commit SHA. It internally pins
`actions/cache` to a SHA (`v5.0.5`).
