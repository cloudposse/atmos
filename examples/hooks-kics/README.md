# `hooks-kics`

Demonstrates the **`kics`** hook kind: a `before-terraform-plan` hook that
runs `kics scan` against the component and renders the SARIF findings
summary in the terminal.

## What this shows

- `kind: kics` with **zero configuration**.
- KICS writes its results into an output **directory** (`results.sarif`
  inside `$ATMOS_OUTPUT_DIR`), unlike most scanners which take a single
  output file path. The kind handles this by reading
  `$ATMOS_OUTPUT_DIR/results.sarif` in its ResultHandler.

## Requirements

- `tofu` on PATH.
- **The `kics` binary auto-installs** via the per-project toolchain
  registry override in this example's `atmos.yaml`. The upstream Aqua
  registry models KICS as `type: go_build` (which the Atmos installer
  doesn't support yet), so we declare it ourselves as a `github_release`
  tarball — the same pattern works for any tool the upstream registry
  doesn't handle well.
- **`KICS_QUERIES_PATH` env var** pointing at the KICS query library.
  KICS's GitHub release tarballs contain *only the binary* — the query
  library is shipped separately. Set it before running:
  - Homebrew: `export KICS_QUERIES_PATH=$(brew --prefix kics)/share/kics/assets/queries`
  - Source clone: `git clone https://github.com/Checkmarx/kics && export KICS_QUERIES_PATH=$(pwd)/kics/assets/queries`
- **No AWS credentials needed** — kics parses HCL directly.

## Cross-platform notes

The toolchain registry override gives you auto-install of the KICS
binary on macOS, Linux, and Windows — all of which have proper KICS
release tarballs (`darwin_amd64`, `darwin_arm64`, `linux_amd64`,
`linux_arm64`, `windows_amd64`, `windows_arm64`). The query library
question is the same on every platform.

## Run

```bash
atmos terraform plan bucket -s test
```

Expected: kics runs before plan and reports findings on the over-permissive
security group and S3 misconfigurations.
