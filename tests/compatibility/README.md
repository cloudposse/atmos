# Template migration compatibility baseline

This suite characterizes **the previous implementation**, then compares actual
old and candidate Atmos binaries on identical inputs. It does not import
`pkg/templating` or use the candidate to compute expected answers.

The baseline is pinned to `4ffd8804d3500b4369e739095174000565b43973`, the main
commit merged into this PR, which still uses gomplate v3. `baseline.json` records
that revision, a SHA-256 digest of the runner and every input, and each CLI
observation. Fixtures are frozen under `tests/fixtures/compatibility`; changes to
unrelated acceptance fixtures cannot silently change this contract.

## Run

Requires Git, tar, and the Go toolchain required by the pinned source. The harness
is a separate **Go standard-library-only module**; old dependencies stay in the
isolated old build. No Python, OpenSSL executable, Terraform process, or live
cloud service is required. Run these commands from the repository root:

```sh
# Test the comparison machinery and local service fixtures.
go -C tests/compatibility test -race ./...

# Build a reusable runner and the candidate.
mkdir -p .context/compatibility
go -C tests/compatibility build -o "$PWD/.context/compatibility/runner" .
go build -trimpath -o .context/compatibility/atmos-candidate .

# Build the pinned old source, run it twice to check stability, then compare
# the candidate with that live oracle on this OS. This is the CI mode.
.context/compatibility/runner verify \
  --candidate .context/compatibility/atmos-candidate \
  --report .context/compatibility/report.json

# Faster local comparison against the checked-in macOS observations.
.context/compatibility/runner compare \
  --candidate .context/compatibility/atmos-candidate \
  --report .context/compatibility/report.json

# Rebuild and replay old source against the checked-in observations.
.context/compatibility/runner baseline
```

On Windows, add `.exe` to the output/executable paths. Exit codes are `0` for
matching observations or approved exceptions, `1` for unapproved differences, and `2` for an invalid
comparison or harness failure. The report contains both sets of observations,
approved and unapproved case IDs, the corpus digest, and candidate binary digest. Printed
diffs show changed fields. `go run .` can also invoke the runner, but wraps its
nonzero exit codes; build the runner to distinguish codes 1 and 2 directly.

The checked-in snapshot was collected on macOS. CI runs `verify` on Linux, macOS,
and Windows, using a fresh old binary on each OS instead of comparing OS-specific
errors to a macOS snapshot. The existing required acceptance checks also require
this compatibility job to pass. Unapproved differences fail the job.

## Extending the baseline

Add cases to `cases.json` or the frozen fixtures, then explicitly record **old**
behavior again:

```sh
go -C tests/compatibility run . baseline --record
```

Recording always builds the pinned Git tree, never the candidate or working tree.
It refuses unstable observations, unexpected old exit codes, missing generated
files, cases that never exercise their declared service, and inputs edited during
collection. Review the baseline diff with its fixture change. Fetch the pinned
commit if a shallow checkout lacks it. Candidate commands cannot record a
baseline. Review baseline changes with the same care as the comparison rules.

## Coverage

All **212 cases** run in fresh fixture directories, with isolated home/config/cache
and controlled environment. Ambient cloud credentials, proxies, and user Atmos
configuration are excluded. The corpus includes:

- Sprig/gomplate collisions and disabled function sets; legacy aliases; valid and
  invalid conversions including empty, nil, overflow, and fractional inputs;
  collection, string, math, encoding, regex, and path operations.
- IP, prefix, and range values, methods, chaining, comparisons, IPv6, invalid
  inputs, and Go types rendered using `printf "%T"`.
- CLI/stack/component settings, custom delimiters, repeated evaluations,
  missing keys, parse errors, and unknown functions.
- JSON, YAML, TOML, CSV, raw includes, escaped file URLs, environment and BoltDB
  datasources, inline definitions, aliases, subpaths, repeated reads, and
  `atmos.GomplateDatasource` caching.
- Local HTTP GET/HEAD, HEAD rejection, redirects, authentication, headers/query
  strings, missing files, malformed data, and timeout behavior.
- SSM strings, SecureString responses, StringList, JSON values, directory
  traversal, missing/denied parameters, session tokens, and invalid credentials.
- S3 object reads, listings, missing objects, and valid/corrupt checksums.
- Consul values, listing shapes, tokens, missing keys, and environment endpoint
  configuration; Vault KV v1/v2, listing, namespace, missing/denied secrets.
- Frozen representative catalogs for stack templates, `!template` native types
  and component references, import context, environment variables, deep locals,
  Atmos Pro template context, stack naming, dev/prod file generation, and docs.

Service fixtures observe request count/order, method, path/query, selected headers,
and SSM bodies. SSM uses a local HTTPS CONNECT endpoint, a short-lived generated
CA, and SigV4 validation with fixture credentials. It never forwards the tunnel
to AWS. S3 also validates signatures. The services are protocol fixtures, not
complete emulators; unsupported SSM operations and CONNECT destinations fail the
harness. Tests verify unsigned requests are rejected and service routes remain
distinct. BoltDB data was generated using the old implementation's libkv storage
format; its reproducible generator is in `tests/compatibility/generate_bolt.go`.

Frozen catalogs originate from these paths at the pinned revision:
`tests/fixtures/scenarios/{stack-templates,atmos-template-yaml-function,import-template-context,template-env-vars,locals-deep-import-chain,atmos-pro-template-regression,stack-name-template}`
and `examples/generate-files`. The import-context fixture adds a CLI config and
component stub to make the original unit fixture runnable. Docs cases are
synthetic fixtures exercising input/inline precedence, environment access, legacy
functions, and generated Markdown.

## Normalization and reviewed exceptions

Only these nondeterministic details are normalized:

- Temporary roots and local service addresses become placeholders; CRLF becomes
  LF. A fixed wide terminal avoids wrapping temporary paths in errors.
- JSON object key order and formatting are ignored. Scalar types, numeric
  spellings, array order, and rendered strings remain observable. Generated file
  contents are compared after path/address/newline normalization.
- Known `Created <filename>` progress lines are sorted because the old generator
  emits them in map iteration order. Duplicates remain significant; other stderr
  stays ordered.

**BoltDB removal is the sole approved compatibility exception**, explicitly
accepted in the migration review on 2026-09-16. The exact cases `boltdb-json`,
`boltdb-plain`, and `boltdb-missing` remain in the corpus. The candidate must exit
1, identify the unsupported `boltdb` scheme, and produce no stdout, generated
files, creation notices, or service requests. Error wrapping and migration
warnings may differ for those three cases. A crash, missing case, unrelated error,
or successful BoltDB read fails this rejection contract. Users must export the
data to a supported datasource, such as a JSON/YAML file, before upgrading.

`exceptions.go` defines this narrow rule. It never applies to old-source
collection, stability checks, or baseline replay; the original BoltDB behavior
remains recorded. No BoltDB adapter will be added to restore support.

For all other cases, exit status, stdout, stderr, generated files, and requests
remain part of the strict contract. Never copy candidate answers into the
baseline. Additional exceptions require review of the exact old/new behavior,
impact, rationale, and migration guidance. See [findings.md](findings.md).

## Limits

This is a substantial bounded regression corpus, not a proof for arbitrary
user templates. Real IAM/KMS, credential-provider chains, cloud pagination,
throttling/retries, TLS failures, Vault renewal, encryption, GCP/Git datasources,
concurrent rendering, remote component outputs, and every gomplate function are
not comprehensively covered. Service fixtures cannot certify live cloud behavior.
Additional production catalogs and integration environments should extend this
suite. Linux/Windows results require actual CI execution; configuring the matrix
does not establish a pass. The migration is not compatible while unapproved
observations remain different.
