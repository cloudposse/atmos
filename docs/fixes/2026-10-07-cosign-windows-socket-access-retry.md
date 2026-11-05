# Retry Windows socket access failures during signature verification

The Windows acceptance job in run `37563863929` failed in
`TestToolchainCustomCommands_ExecuteWithDependencies/test-gum`. Cosign could not
connect to Rekor while verifying gum's checksum signature:

```text
dial tcp 34.36.47.134:443: connectex: An attempt was made to access a socket in a way forbidden by its access permissions.
```

`rekor.sigstore.dev:443` was already in the job's egress allowlist, and the runner
logged allow rules for the same IP. This is consistent with the Windows network
enforcement flakes described in `2026-09-07-jit-source-network-flakes.md`; it is
a transport failure before a signature verdict. The exact runner failure cannot
be reproduced locally on macOS.

The signature-verification retry classifier now recognizes this specific Windows
socket error. It uses the existing five-attempt exponential-backoff budget and
repeats verification with the same arguments. Persistent denial still returns an
error; signature verification and the runner's egress policy remain enforced.
File access and verifier execution permission errors remain non-retryable.

Regression tests reproduce the logged error, verify recovery after one denied
connection, verify failure after the retry budget is exhausted, and check that
ordinary permission failures are not classified as retryable.
