# CI platform dependencies and cache retention

The first attempt of Tests run 35229465949 took 28m36s. The Intel Mac build
finished at 11m42s, and every job declaring `needs: build` waited for it, even
though Linux was ready at 4m46s. Floci then ran for 16m07s. The Intel Go cache
was a hit: downloading its 2.94 GB archive took about 76 seconds and extracting
it took another 4m43s.

## Changes

- Build jobs and their acceptance, registry, mock, and k3s consumers have
  platform-specific dependencies. Required acceptance checks retain their names
  and validate their own platform. Coverage waits only for Linux and Mage tests.
- The ARM Mac builds the Intel CLI with the existing `macos-intel` target.
  `ATMOS_BUILD_OUTPUT` optionally overrides the build output path; leaving it
  unset preserves `build/atmos` or `build/atmos.exe`. The native artifact is
  uploaded first, and Intel output lives in a separate temporary directory.
  Both artifact names remain unchanged. Intel Colima execution is retained.
- Mac cache warmup builds both architectures. Linux race objects use a separate
  cache from regular compilation. Warmup regenerates build objects from the
  current graph, retaining reusable module downloads. Floci's pure-Go test
  compilation is warmed explicitly, and its tests run in two isolated groups.
- Colima receives the Intel host's available CPUs and 6 GiB of memory. Installed
  tools are reused; existing bounded startup retries and diagnostics remain.
- Local and nested action references use `$/`. The timing-summary, PR-size,
  and Dependabot-label jobs no longer check out source only to load an action.
  Jobs that read source, configuration, or Git history still check out source.

## Cache retention

The cache inventory held 98.5 GiB against a configured 100 GB limit. Successful
warmups saved a new immutable generation nightly, while older generations waited
for GitHub's general eviction policy. A dry run of the new pruning policy found
nine redundant main-branch caches totaling 28.4 GiB.

After a successful main-branch warmup, the cleanup action retains the latest two
entries for each exact cache family/OS/architecture/Go-version/dependency-hash
combination. Entries less than 24 hours old are additionally protected. It
inventories all pages before deletion and never selects PR caches, unrelated
cache families, or malformed keys. Inventory or deletion failures fail the
cleanup job. Manual action use defaults to dry-run mode.

Pre-commit and CodeQL Go setup now restore the shared cache rather than saving
additional Go archives on PR refs. The repository storage limit is unchanged.
No live caches were deleted during implementation; cleanup begins when the
updated warmup runs on main. New race caches and Intel objects in the ARM cache
must warm before their restore savings are realized.

## Validation and rollout

Go tests cover output isolation, platform dependencies, all thirty acceptance
shards, required-check names, and exact Floci test routing. Node tests cover
cache-family boundaries, generation retention, grace periods, pagination,
dry-run behavior, and API failure handling. A real ARM-to-Intel compilation
produced a Mach-O x86_64 executable.

The installed actionlint 1.7.12 predates self-repository syntax. Validation
normalizes only `uses: $/` to `uses: ./` in memory before linting; files retain
the new syntax. The sampled RunsOn runner is 2.337.0, above the 2.336.0 minimum.

After merging, inspect a successful cache warmup and subsequent complete PR
runs. Compare total elapsed time, platform build completion, Floci durations,
Colima startup, cache-hit/restore duration, and retained cache bytes. A 20-minute
end-to-end result must be measured in hosted CI; local checks cannot establish
runner queue times or Intel virtualization performance.
