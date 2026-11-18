
## Artifacts

1 operation: 1 succeeded.

<!-- markdownlint-disable MD055 MD056 -->
| # | Operation | Status | Artifact | Repository | Destination | Digest | Size | Duration |
| ---: | --- | --- | --- | --- | --- | --- | ---: | ---: |
| 1 | Sign | ✅ Signed | `components/vpc:1.2.0` | `shared` |  | `sha256:bbbbbbbbbbbb…` (record) |  | 300 ms |
<!-- markdownlint-enable MD055 MD056 -->

### ✅ 1. Sign `components/vpc:1.2.0`

Signed `components/vpc:1.2.0` in repository `shared`.

- **Status:** Signed
- **Step:** sign
- **Repository:** `shared`
- **Reference:** `components/vpc:1.2.0`
- **Duration:** 300 ms
- **Lock:** repository scope. Writers on every host were serialized through the repository's own coordination.
- **Signature:** signed. Signature evidence was stored for this subject. Signing alone does not authenticate the signer: verify the artifact against a trust policy to do that.
  - Subject: record `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`
  - Evidence: sigstore-bundle `sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee`

<details>
<summary>Digests (1)</summary>

| Type | Digest | Identifies |
| --- | --- | --- |
| Record | `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` | canonical bytes of the package Record |

Digests prove that the content is intact. They do not authenticate the publisher, and this operation did not report a verified signature.

</details>
