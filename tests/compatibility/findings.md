# Migration compatibility comparison

Baseline: `4ffd8804d3500b4369e739095174000565b43973` (gomplate v3).
Candidate: this branch, compared in CI with `runner verify` on Linux, macOS and
Windows, which builds the pinned old source on each OS. `runner compare` gives
the same result locally against the checked-in macOS baseline.

The old implementation reproduces **212/212 cases** across repeated runs. The
candidate matches **158/212 cases**; the other **54 differences are approved**
(`boltdb-json` and `boltdb-plain` below, plus the 52 recorded observations in
`approved.json`; `boltdb-missing` failed before too, so it counts as a match)
and **0 remain unapproved**, so the gate passes on all three platforms. These
numbers count cases, not distinct root causes.

## Comparison rules

Candidate comparisons ignore stderr: error formatting, template line numbers and
deprecation warnings change between releases without changing behavior. Exit
codes, stdout, generated files, file notices and service requests must still
match. Old-source recording, stability checks and replay compare every field,
including stderr.

An approved change is an exact recorded observation in `approved.json`: exit
code, stdout and generated files, plus the reason. Requests must match the old
observation unless the entry records the approved new requests.

## Approved removal: BoltDB

The migration review explicitly accepted dropping BoltDB support on 2026-09-16.
No compatibility adapter is planned. The `boltdb-json` and `boltdb-plain` cases
previously read data successfully; the candidate now rejects the unsupported
scheme. `boltdb-missing` previously failed on a missing key and now rejects the
scheme. All three remain tested: exit 1, an explicit unsupported-BoltDB diagnostic,
empty stdout, and no generated files or service requests are required. Diagnostic
wrapping and migration warnings are outside the compatibility contract for this
removed feature. Old baseline observations remain unchanged.

Users of BoltDB datasources must export their data to a supported datasource,
such as a JSON/YAML file, before upgrading. This exception applies only to the
three named cases; other datasource failures remain blocking.

## Approved behavior changes

The migration review accepted these documented v5 changes on 2026-11-06. Each
appears in the v5 changelog. Entries with recorded requests approve that exact
traffic; all others require the old requests.

| Cases | Approved difference |
| --- | --- |
| `conv-atoi-invalid`, `conv-int-invalid`, `conv-int64-invalid`, `conv-float-invalid`, `convert-toint64-empty`, `convert-toint64-nil`, `convert-tofloat64-empty`, `convert-tofloat64-nil` | Invalid, empty and nil conversions return an error instead of a silent zero value. |
| `subpath-no-slash` | A datasource sub-path resolves as a relative URL; directory datasources must end with `/`. |
| `ssm-string`, `ssm-json`, `ssm-json-type`, `ssm-secure`, `ssm-stringlist`, `ssm-session-token`, `ssm-value-field` | `aws+smp://` returns the parameter value directly instead of an object with `Value` and metadata. |
| `consul-directory`, `consul-directory-fields` | A `consul://` directory listing returns key names instead of key/value objects; the listing requests also differ. |
| `consul-denied`, `consul-env-address`, `consul-json`, `consul-missing`, `consul-plain`, `vault-denied`, `vault-directory`, `vault-kv1`, `vault-missing`, `vault-namespace`, `ssm-directory`, `ssm-missing`, `s3-object`, `s3-missing` | Same exit code and output; v5's Consul, Vault and AWS SDK v2 clients send different requests (consistency parameters, Vault mount probing, directory probes, SDK query parameters). The recorded requests are approved. |
| `s3-corrupt-checksum` | An S3 object whose checksum does not match is rejected instead of returned. |
| `env-datasource` | `env://NAME` read no variable in v3 (`NAME` parses as the URL host, so v3 read the variable named `""` and returned empty data); v5 fails. Use `env:NAME` or `env:///NAME`. |
| `vault-kv2-raw` | v5 detects KV v2 mounts and adds `/data/` itself, so a URL that already includes `/data/` fails. Drop `/data/`; v5 returns the secret's data without the metadata envelope. |
| `http-timeout` | The global `templates.settings.gomplate.timeout` applies when the stack manifest sets none. Older releases replaced it with the 5-second default, so this case's 1-second timeout was ignored and its 2-second read succeeded; it now times out. |
| `coll-jq`, `tmpl-inline`, `tmpl-inline-datasource` | Failed in v3 because the function did not exist; work in v5. |
| 13 `catalog-*` cases | `describe component` output gains empty `generate`, `provision` and `source` sections from newer Atmos; template-rendered values are unchanged. |

## Restored by compatibility shims

- **Bare `slice` with Sprig disabled:** gomplate v3 registered `coll.Slice` under
  this name. The shim restores it, with a warning, only when Sprig is disabled.
- **Network values:** `net.ParseIP`, `net.ParseIPPrefix` and `net.ParseIPRange`
  return `netaddr.IP`, `netaddr.IPPrefix` and `netaddr.IPRange` again
  (`pkg/template/compatibility/netaddr`), with v3's method set, so method calls,
  chaining and `printf "%T"` match.
- **HTTP datasources:** reads use a single GET, as in v3. Gomplate v5 sent a HEAD
  first and failed against servers that reject HEAD (`http-no-head`); every other
  `http-*` case also matches again.

## Review policy and reproducibility

For every remaining difference, restore the old behavior or review an explicit exception
with exact old/new observations, user impact, rationale, and migration guidance.
Do not replace the old baseline with the candidate's answers. Exceptions are the BoltDB rule and the
recorded observations in `approved.json`; stderr is the only field ignored.

[README.md](README.md) gives reproducible commands, normalization rules, provenance,
and uncovered paths. `baseline.json` contains the exact old observations. The
runner's `--report` contains both old and candidate observations plus binary/input
digests; CI uploads that report per OS. Service fixtures are bounded and do not
replace live-service integration testing or representative user catalogs.

The Go port was checked against the previous experimental runner: every unchanged
fixture produced identical old and candidate observations. The intentionally
corrected basic docs fixture now succeeds in both; the old `.Env` expression is
retained separately as a negative case. No compatibility result was waived during
the port.
