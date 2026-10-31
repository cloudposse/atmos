
## Artifacts

1 operation: 1 failed.

<!-- markdownlint-disable MD055 MD056 -->
| # | Operation | Status | Artifact | Repository | Destination | Digest | Size | Duration |
| ---: | --- | --- | --- | --- | --- | --- | ---: | ---: |
| 1 | Verify | ❌ Failed | `components/vpc:1.2.0` | `shared` |  | `sha256:bbbbbbbbbbbb…` (record) |  |  |
<!-- markdownlint-enable MD055 MD056 -->

### ❌ 1. Verify `components/vpc:1.2.0`

The verify operation for `components/vpc:1.2.0` failed.

- **Status:** Failed
- **Step:** verify
- **Repository:** `shared`
- **Reference:** `components/vpc:1.2.0`
- **Signature:** failed. Required verification did not succeed. The artifact must not be trusted.
  - Subject: record `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`
  - Policy fingerprint: `sha256:ffffffffffff…`

**Error**

```text
artifact signature verification failed: wrong issuer
```

<details>
<summary>Digests (1)</summary>

| Type | Digest | Identifies |
| --- | --- | --- |
| Record | `sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb` | canonical bytes of the package Record |

Digests prove that the content is intact. They do not authenticate the publisher, and this operation did not report a verified signature.

</details>
