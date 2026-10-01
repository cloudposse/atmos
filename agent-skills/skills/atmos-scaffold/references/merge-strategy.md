# 3-Way Merge Reference

`--update` re-runs a template against an existing, previously-generated (or
otherwise pre-populated) target directory instead of refusing to write into a
non-empty directory. It performs a real git-based 3-way merge per file: base
(the version the target was originally generated from), theirs (the freshly
rendered template output), and ours (the file currently on disk).

## Flags

```shell
atmos scaffold generate my-template ./target --update
atmos scaffold generate my-template ./target --update --base-ref=v1.2.0
atmos scaffold generate my-template ./target --update --merge-strategy=manual
atmos scaffold generate my-template ./target --update --merge-strategy=ours
atmos scaffold generate my-template ./target --update --merge-strategy=theirs
atmos scaffold generate my-template ./target --update --dry-run
```

- `--base-ref`: the git ref in the target directory to use as the merge base. Defaults
  to `HEAD` when `--update` is set and no ref is given — `atmos scaffold generate --git`
  always creates an initial commit, so `HEAD` is a sensible default base.
- `--merge-strategy`: conflict resolution when ours and theirs both changed the same
  region relative to base.
  - `manual` (default) — surface conflicts (conflict markers in the written file); the
    command still succeeds, the operator resolves markers by hand.
  - `ours` — keep the on-disk version for conflicting regions.
  - `theirs` — take the freshly rendered template version for conflicting regions.
- `--dry-run` with `--update`: runs the *real* merge path — base load, 3-way merge, and
  conflict detection all execute — but nothing is written to disk. This is the only way
  to get an accurate create/update/conflict preview under `--update`; without `--update`,
  `--dry-run` alone just renders a simpler path-only preview.

## Base storage

The merge base is loaded from git (via `SetupGitStorage`, keyed by `--base-ref`) into
the target directory's processor before any file is touched. If a file existed in the
base but the template no longer generates it, or vice versa, the merge degrades
gracefully to a two-way comparison for that file (no common ancestor).

## Merge threshold

The merger has an internal conflict-percentage threshold (currently a hardcoded 50%
default) — if too much of a file's content conflicts, the merge for that file fails
outright rather than emitting a heavily-marked-up result. There is no CLI flag to
override this threshold today (`--max-changes` does not exist despite older docs
mentioning it as planned).

## The "offer to update instead of failing" prompt

When `--update`/`--force` are **not** set and the target directory is non-empty,
`atmos scaffold generate` fails with `ErrTargetDirectoryNotEmpty` — but if running
interactively (a real TTY), it offers to retry the same generation with `--update`
implied instead of just failing (`ConfirmUpdateInstead`). Declining leaves the original
failure in place; confirming retries with `update: true` and the base ref defaulted to
`HEAD` if none was given. This behavior is shared verbatim between `atmos scaffold
generate` and `atmos init` (`shouldOfferScaffoldUpdate`/`shouldOfferUpdate`).

## `--force` vs `--update`

If both are set, `--update` takes precedence: existing files go through the 3-way merge
path, not a raw overwrite. `--force` alone (without `--update`) overwrites files
unconditionally with no merge at all.
