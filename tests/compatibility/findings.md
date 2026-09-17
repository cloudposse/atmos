# Migration compatibility comparison — review pending

Baseline: `4ffd8804d3500b4369e739095174000565b43973` (gomplate v3).
Candidate implementation: `d18dee6386ddd9da28c60b18b9c49eb11df453d0`.
Environment: Go 1.27.1, macOS/arm64. Harness: Go, standard library only.

The old implementation reproduced **212/212 cases** across repeated runs. The
candidate matches **85/212 cases**; **127 cases have unapproved differences**.
In **32 cases**, an old success becomes a candidate failure. These numbers count
cases, not distinct root causes. Warnings, errors, types, and requests are part of
the comparison, so a difference does not necessarily mean a rendered value broke.

All 13 frozen catalog-description cases, both dev/prod file-generation cases, and
the basic docs-generation case match. That alone does **not** establish backward
compatibility. The migration comparison is currently a failing gate, with no
approved exceptions. Linux/Windows runs still need CI execution.

## Previously successful cases that now fail

| Cases | Observed difference |
| --- | --- |
| `conv-atoi-invalid`, `conv-int-invalid`, `conv-int64-invalid`, `conv-float-invalid` | Invalid conversion previously rendered zero; now raises an error. |
| `convert-toint64-empty`, `convert-toint64-nil`, `convert-tofloat64-empty`, `convert-tofloat64-nil` | Empty/nil conversions previously succeeded; now raise errors. |
| `bare-slice-sprig-disabled` | Legacy slice call no longer accepts the old arguments. |
| `ip-chain-prior`, `ip-method-compare`, `ip-method-less` | Legacy IP method chaining/comparison no longer accepts the old values or method set. |
| `prefix-contains`, `prefix-method-ip`, `prefix-method-ipnet`, `prefix-method-iszero`, `prefix-method-range`, `prefix-method-valid`, `range-contains` | Prefix/range method names or argument types are incompatible. |
| `subpath-no-slash` | A datasource directory without a trailing slash resolves differently. |
| `http-no-head` | A server that supports GET but returns 501 for HEAD is no longer usable. |
| `boltdb-json`, `boltdb-plain` | Previously supported BoltDB datasource reads fail. |
| `env-datasource` | An environment datasource read that succeeds in the old CLI fails. |
| `ssm-value-field`, `ssm-secure`, `ssm-json`, `ssm-stringlist`, `ssm-session-token` | Existing templates accessing parameter metadata/`.Value` fail against the changed return shape. |
| `consul-directory-fields` | Access to the old listing's key/value fields fails. |
| `vault-kv2-raw` | The same raw KV-v2 endpoint expression fails in the candidate. |
| `s3-corrupt-checksum` | The candidate rejects a corrupt checksum that the old implementation accepts. This may be a desirable integrity improvement, but requires explicit review as an intended difference. |

These results characterize the exact fixture expressions and protocols; they do
not imply that every use of SSM, Vault, or an affected function fails.

## Other observable differences

- **SSM shape:** `ssm-string` changes from a parameter object containing `Value`,
  `Name`, `Type`, and metadata to the value string alone. Both exit successfully.
  The directory fixture also observes additional probes/reads. Requests use
  verified SigV4 with fixture credentials; no live IAM/KMS behavior is implied.
- **Consul shape and requests:** a directory previously returned an array of
  `{key,value}` objects; the candidate returns an array of names. The fixture
  observes one recursive request become five requests. Other reads also change
  consistency query parameters. Values that still render correctly can therefore
  have different backend semantics and latency.
- **HTTP/S3/Vault access:** additional HEAD/probe/list requests and changed errors
  remain visible. For the configured one-second HTTP timeout, both versions still
  complete the two-second fixture response; the candidate adds HEAD. This case
  characterizes observed timeout behavior, not a guarantee of timeout enforcement.
- **Network values:** `netaddr.IP`, `netaddr.IPPrefix`, and `netaddr.IPRange` become
  `templating.legacyIP`, `netip.Prefix`, and `netipx.IPRange`. Matching printed
  addresses does not establish method or type compatibility. Many network cases
  also gain deprecation warnings.
- **Diagnostics:** legacy aliases and docs templates gain warnings. Some negative
  cases continue to fail but produce different errors. They remain unapproved
  differences even when exit status stays the same.

## Review policy and reproducibility

For every difference, restore the old behavior or review an explicit exception
with exact old/new observations, user impact, rationale, and migration guidance.
Do not replace the old baseline with the candidate's answers. No exception
mechanism or blanket warning suppression has been introduced.

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

## Complete difference inventory

This inventory identifies every differing case and observation field from the
212-case comparison. It is a review checklist, not an approved exception list.
Exit columns show old → candidate status.

| Case | Exit | Changed fields |
| --- | --- | --- |
| `atmos-datasource` | 0 → 0 | requests |
| `atmos-datasource-cached` | 0 → 0 | requests |
| `bare-slice-sprig-disabled` | 0 → 1 | exit_code, stdout, stderr |
| `bare-splitn-legacy-sprig-enabled` | 0 → 0 | stderr |
| `bare-splitn-sprig-disabled` | 0 → 0 | stderr |
| `boltdb-json` | 0 → 1 | exit_code, stdout, stderr |
| `boltdb-missing` | 1 → 1 | stderr |
| `boltdb-plain` | 0 → 1 | exit_code, stdout, stderr |
| `coll-jq` | 1 → 0 | exit_code, stdout, stderr |
| `consul-denied` | 1 → 1 | stderr, requests |
| `consul-directory` | 0 → 0 | stdout, requests |
| `consul-directory-fields` | 0 → 1 | exit_code, stdout, stderr, requests |
| `consul-env-address` | 0 → 0 | requests |
| `consul-json` | 0 → 0 | requests |
| `consul-missing` | 1 → 1 | stderr, requests |
| `consul-plain` | 0 → 0 | requests |
| `conv-atoi-invalid` | 0 → 1 | exit_code, stdout, stderr |
| `conv-bool` | 0 → 0 | stderr |
| `conv-dict` | 0 → 0 | stderr |
| `conv-dict-odd` | 0 → 0 | stderr |
| `conv-float-invalid` | 0 → 1 | exit_code, stdout, stderr |
| `conv-has` | 0 → 0 | stderr |
| `conv-int-invalid` | 0 → 1 | exit_code, stdout, stderr |
| `conv-int64-invalid` | 0 → 1 | exit_code, stdout, stderr |
| `conv-slice` | 0 → 0 | stderr |
| `convert-tofloat64-empty` | 0 → 1 | exit_code, stdout, stderr |
| `convert-tofloat64-nil` | 0 → 1 | exit_code, stdout, stderr |
| `convert-toint64-empty` | 0 → 1 | exit_code, stdout, stderr |
| `convert-toint64-nil` | 0 → 1 | exit_code, stdout, stderr |
| `docs-env-context` | 1 → 0 | exit_code, stderr |
| `docs-legacy` | 0 → 0 | stderr |
| `env-datasource` | 0 → 1 | exit_code, stdout, stderr |
| `file-not-found` | 1 → 1 | stderr |
| `http-authorized` | 0 → 0 | requests |
| `http-get-only` | 0 → 0 | requests |
| `http-headers-query` | 0 → 0 | requests |
| `http-json` | 0 → 0 | requests |
| `http-no-head` | 0 → 1 | exit_code, stdout, stderr, requests |
| `http-not-found` | 1 → 1 | stderr, requests |
| `http-redirect` | 0 → 0 | requests |
| `http-repeated-read` | 0 → 0 | requests |
| `http-subpath` | 0 → 0 | requests |
| `http-timeout` | 0 → 0 | requests |
| `http-unauthorized` | 1 → 1 | stderr, requests |
| `http-yaml` | 0 → 0 | requests |
| `ip-chain-prior` | 0 → 1 | exit_code, stdout, stderr |
| `ip-method-as16` | 0 → 0 | stderr |
| `ip-method-as4` | 0 → 0 | stderr |
| `ip-method-bitlen` | 0 → 0 | stderr |
| `ip-method-compare` | 0 → 1 | exit_code, stdout, stderr |
| `ip-method-ipaddr` | 0 → 0 | stderr |
| `ip-method-is4` | 0 → 0 | stderr |
| `ip-method-is4in6` | 0 → 0 | stderr |
| `ip-method-is6` | 0 → 0 | stderr |
| `ip-method-isglobalunicast` | 0 → 0 | stderr |
| `ip-method-isinterfacelocalmulticast` | 0 → 0 | stderr |
| `ip-method-islinklocalmulticast` | 0 → 0 | stderr |
| `ip-method-islinklocalunicast` | 0 → 0 | stderr |
| `ip-method-isloopback` | 0 → 0 | stderr |
| `ip-method-ismulticast` | 0 → 0 | stderr |
| `ip-method-isprivate` | 0 → 0 | stderr |
| `ip-method-isunspecified` | 0 → 0 | stderr |
| `ip-method-isvalid` | 0 → 0 | stderr |
| `ip-method-iszero` | 0 → 0 | stderr |
| `ip-method-less` | 0 → 1 | exit_code, stdout, stderr |
| `ip-method-next` | 0 → 0 | stderr |
| `ip-method-prior` | 0 → 0 | stderr |
| `ip-method-string` | 0 → 0 | stderr |
| `ip-method-stringexpanded` | 0 → 0 | stderr |
| `ip-method-unmap` | 0 → 0 | stderr |
| `ip-prefix-method` | 0 → 0 | stderr |
| `ip-v6-mapped` | 0 → 0 | stderr |
| `ip-v6-zone` | 0 → 0 | stderr |
| `malformed-datasource` | 1 → 1 | stderr |
| `net-ip` | 0 → 0 | stdout, stderr |
| `net-ip-invalid` | 1 → 1 | stderr |
| `net-prefix` | 0 → 0 | stdout, stderr |
| `net-range` | 0 → 0 | stdout, stderr |
| `prefix-contains` | 0 → 1 | exit_code, stdout, stderr |
| `prefix-invalid` | 1 → 1 | stderr |
| `prefix-method-bits` | 0 → 0 | stderr |
| `prefix-method-ip` | 0 → 1 | exit_code, stdout, stderr |
| `prefix-method-ipnet` | 0 → 1 | exit_code, stdout, stderr |
| `prefix-method-issingleip` | 0 → 0 | stderr |
| `prefix-method-isvalid` | 0 → 0 | stderr |
| `prefix-method-iszero` | 0 → 1 | exit_code, stdout, stderr |
| `prefix-method-masked` | 0 → 0 | stderr |
| `prefix-method-range` | 0 → 1 | exit_code, stdout, stderr |
| `prefix-method-string` | 0 → 0 | stderr |
| `prefix-method-valid` | 0 → 1 | exit_code, stdout, stderr |
| `prefix-overlaps` | 0 → 0 | stderr |
| `range-contains` | 0 → 1 | exit_code, stdout, stderr |
| `range-invalid` | 1 → 1 | stderr |
| `range-method-from` | 0 → 0 | stderr |
| `range-method-isvalid` | 0 → 0 | stderr |
| `range-method-iszero` | 0 → 0 | stderr |
| `range-method-prefixes` | 0 → 0 | stderr |
| `range-method-string` | 0 → 0 | stderr |
| `range-method-to` | 0 → 0 | stderr |
| `range-method-valid` | 0 → 0 | stderr |
| `range-overlaps` | 0 → 0 | stderr |
| `s3-corrupt-checksum` | 0 → 1 | exit_code, stdout, stderr, requests |
| `s3-missing` | 1 → 1 | stderr, requests |
| `s3-object` | 0 → 0 | requests |
| `sprig-disabled-aliases` | 0 → 0 | stderr |
| `ssm-denied` | 1 → 1 | stderr |
| `ssm-directory` | 0 → 0 | requests |
| `ssm-invalid-credentials` | 1 → 1 | stderr |
| `ssm-json` | 0 → 1 | exit_code, stdout, stderr |
| `ssm-json-type` | 0 → 0 | stdout |
| `ssm-missing` | 1 → 1 | stderr, requests |
| `ssm-secure` | 0 → 1 | exit_code, stdout, stderr |
| `ssm-session-token` | 0 → 1 | exit_code, stdout, stderr |
| `ssm-string` | 0 → 0 | stdout |
| `ssm-stringlist` | 0 → 1 | exit_code, stdout, stderr |
| `ssm-value-field` | 0 → 1 | exit_code, stdout, stderr |
| `strings-sort` | 0 → 0 | stderr |
| `subpath-no-slash` | 0 → 1 | exit_code, stdout, stderr |
| `tmpl-inline` | 1 → 0 | exit_code, stdout, stderr |
| `tmpl-inline-datasource` | 1 → 0 | exit_code, stdout, stderr |
| `unknown-datasource` | 1 → 1 | stderr |
| `vault-denied` | 1 → 1 | stderr, requests |
| `vault-directory` | 0 → 0 | requests |
| `vault-kv1` | 0 → 0 | requests |
| `vault-kv2-raw` | 0 → 1 | exit_code, stdout, stderr, requests |
| `vault-missing` | 1 → 1 | stderr, requests |
| `vault-namespace` | 0 → 0 | requests |
