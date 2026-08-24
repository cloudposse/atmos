# Podman Port Read-Back Dropped, Breaking Emulator Endpoint Resolution

**Date:** 2026-06-27
**Severity:** High — every emulator-backed `terraform` operation under the Podman runtime silently targeted the **real cloud** instead of the local emulator
**Reproducer:** `pkg/container/podman_test.go` (`TestParsePodmanPorts`, `TestParsePodmanContainer_PopulatesPorts`)

---

## Why this is a fix doc (and not a blog post / changelog entry)

This corrects a regression in existing behavior (the emulator feature was already shipped) and
introduces **no net-new user-facing capability** — no new command, flag, or config. Per the repo's
label decision tree that is a bug fix, which does not require a `website/blog/` post or a roadmap
milestone. The PR that carries it is labeled `minor` for an unrelated net-new feature (local
Terraform tests against emulators); this endpoint fix rides along because that feature cannot work
under Podman without it. The rationale is captured here in `docs/fixes/` instead.

---

## Symptom

With the Podman container runtime, bringing up an AWS emulator and running Terraform against it hit
real AWS instead of the local sandbox:

```text
Error: creating S3 Bucket (demo-dev): operation error S3: CreateBucket,
  https response error StatusCode: 403, ... api error InvalidAccessKeyId:
  The AWS Access Key Id you provided does not exist in our records.
Error: creating AWS DynamoDB Table (...): UnrecognizedClientException:
  The security token included in the request is invalid.
```

`atmos emulator up aws -s <stack>` printed `✓ emulator aws is up` (note: **not** `...is up at
http://localhost:PORT`), and the generated `providers_override.tf.json` contained the dummy creds,
`s3_use_path_style`, and skip-flags but **no `endpoints` block**. The container itself was healthy
with a published port (`podman ps` showed `0.0.0.0:35853->4566/tcp`).

Docker was unaffected — this reproduced only under Podman.

---

## Root Cause

`parsePodmanContainer` (`pkg/container/podman.go`) built the container `Info` from `podman ps
--format json` but **never populated `info.Ports`** — it parsed ID, name, image, status, health, and
labels only. Docker's parser (`parseDockerContainer`) reads ports from Docker's `.Ports` string
column; Podman's structured `Ports` array was silently dropped.

The empty `info.Ports` then cascaded:

1. `Manager.endpoint()` (`pkg/emulator/manager.go`) builds the emulator `Endpoint.Ports` map from
   `container.FindInstance(...).Ports`. `FindInstance` → `runtime.List` → `ps -a --format json` →
   `parsePodmanContainer`, so the dropped ports left the map empty.
2. An empty ports map means `Endpoint.URL("http")` returns `""`.
3. `AWSProfile` (`pkg/emulator/target/aws.go`) gates **both** `AWS_ENDPOINT_URL` (subprocess env) and
   the provider `endpoints` fragment on `if url != ""`. With an empty URL, neither was injected.
4. Terraform therefore used the default (real) AWS endpoint with the dummy `test/test` credentials →
   `403 InvalidAccessKeyId`.

This affected all emulator-backed Terraform under Podman, not just `terraform test`.

---

## The Fix

Add `parsePodmanPorts` and wire it into `parsePodmanContainer`
(`pkg/container/podman.go`). Podman's `ps --format json` represents ports as a structured array with
**snake_case** keys (`{host_ip, container_port, host_port, range, protocol}`, numbers decoding to
`float64`), unlike Docker's flat string. The parser:

- reads `container_port` / `host_port` / `protocol` via the existing `getInt64` helper,
- defaults the protocol to `tcp` when absent,
- skips entries with no published host port (`host_port == 0`),
- expands a `range > 1` mapping into consecutive container/host port pairs.

`FindInstance` uses the `List` (`ps`) path, so populating ports there is sufficient — no inspect-path
change is required.

### Verification

```text
✓ emulator aws is up at http://localhost:37223        # was "is up" (empty URL)
providers_override.tf.json now contains the "endpoints" block
atmos terraform test app -s local → Success! 3 passed, 0 failed.   # was 1 passed, 1 failed
```

---

## Files

| File | Change |
| --- | --- |
| `pkg/container/podman.go` | Add `parsePodmanPorts`; populate `info.Ports` in `parsePodmanContainer`. |
| `pkg/container/podman_test.go` | `TestParsePodmanPorts` (single/default-protocol/range/unpublished/wrong-type cases) + `TestParsePodmanContainer_PopulatesPorts` regression guard. |

## Related

- `pkg/emulator/manager.go` — `endpoint()` consumes `info.Ports`.
- `pkg/emulator/target/aws.go` — `AWSProfile` gates the endpoint env var and provider fragment on a non-empty URL.
