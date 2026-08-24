# Terraform Registry Cache — HTTPS / Self-Signed TLS (Research + Design)

## Problem

The cache proxy serves over plain HTTP (`http://127.0.0.1:<port>/`, `pkg/http/proxy/server.go`)
and injects that URL as a `provider_installation { network_mirror { url = ... } }` directive
(`pkg/terraform/cache/cache.go`). Both Terraform and OpenTofu **reject non-`https` network
mirrors**:

```
Error: Invalid URL for provider installation source
Cannot use "http://127.0.0.1:50782/providers/" as a URL for a network provider
mirror: the mirror must be at an https: URL.
```

So the proxy must serve **HTTPS**, and the `terraform`/`tofu` subprocess must **trust** the
proxy's certificate. The cache is loopback-only and ephemeral, so a self-signed cert is the
natural choice — the open question is **how the subprocess trusts it, per platform**.

## Research findings (the decisive constraint)

Terraform and OpenTofu are Go programs and use Go's `crypto/x509` system trust:

| Platform | `SSL_CERT_FILE` / `SSL_CERT_DIR` honored? | How to trust a local self-signed cert |
| --- | --- | --- |
| **Linux / *BSD** | **Yes** (`root_unix.go`) — but they **replace** defaults, they don't append | Point `SSL_CERT_FILE` at a bundle = **system roots + our cert** |
| **macOS** | **No** — uses the platform verifier (keychain/`trustd`); env vars ignored | Install the cert into the **login keychain** (`security add-trusted-cert`) |
| **Windows** | **No** — uses the platform verifier (system cert store) | Install into the **Windows cert store** |

Sources:
- [root_unix.go](https://go.dev/src/crypto/x509/root_unix.go) — `SSL_CERT_FILE` **replaces** `certFiles`; `SSL_CERT_DIR` **replaces** `certDirectories`; build tag excludes darwin.
- [golang/go#77865](https://github.com/golang/go/issues/77865) — proposal to honor `SSL_CERT_FILE` on darwin is **accepted but unimplemented** as of 2026.
- [crypto/x509 docs](https://pkg.go.dev/crypto/x509) — darwin/windows use the platform verifier unless `GODEBUG=x509usefallbackroots=1` **and** the program called `x509.SetFallbackRoots` (terraform/tofu do **not**, so this path yields an empty root set — not viable).

**Headline:** the env-var trust trick works on **Linux/CI** but **not on macOS/Windows**. On
those platforms trust requires writing to the OS trust store, which is invasive and not something
a transparent cache should do silently.

### Important `SSL_CERT_FILE` nuance (Linux)
Setting `SSL_CERT_FILE` to *only* our cert would drop the system roots and break every other TLS
the subprocess does (state backends like S3, module `git::https`, provider `direct` fallback). The
bundle **must** be `system roots + our cert`. Getting system roots as PEM cross-platform is the
fiddly part (read the existing `SSL_CERT_FILE`/Go's known `certFiles` locations on Linux; there is
no PEM file on macOS).

## Recommended design

1. **HTTPS proxy with a cached self-signed CA.**
   - Generate a self-signed cert with SANs `127.0.0.1`, `::1`, `localhost` (ECDSA P-256).
   - **Cache it** at `<cacheRoot>/tls/` (`ca.pem`, `ca-key.pem`), reuse across runs, regenerate
     only when missing or near expiry (long-lived, e.g. 10y CA + short leaf, or a single
     long-lived cert for simplicity). Key file mode `0600`.
   - `pkg/http/proxy/server.go` serves `https://` via `ServeTLS`; `cache.go` injects an `https://`
     `network_mirror` URL.

2. **Startup emit (requested).**
   - First generation: `ui.Success("Generated Terraform registry cache certificate (<path>)")`.
   - Every run: `ui.Success("Registry cache proxy listening on https://127.0.0.1:<port>")`
     (already added for the current HTTP server; update the scheme when HTTPS lands).

3. **Trust wiring, per platform.**
   - **Linux/CI:** write `<cacheRoot>/tls/bundle.pem` = system roots + our cert, set
     `SSL_CERT_FILE` (and `SSL_CERT_DIR`) in the subprocess env. Defer if the user already manages
     `SSL_CERT_FILE`.
   - **macOS/Windows:** an **opt-in** `atmos terraform cache trust` command (mkcert-style) that
     installs the CA into the login keychain / cert store, with a matching `cache untrust`. The
     transparent cache never modifies the OS trust store without this explicit command.

4. **Dogfood:** `atmos terraform cache mirror` continues to run **through** the proxy (per product
   direction) — it exercises the same HTTPS path as a normal `init`.

## Open decision for the team
- Accept the **opt-in keychain trust** model for macOS/Windows (vs. only supporting CI/Linux
  transparently)? This is the only way to make the proxy work on developer macOS.
- Single long-lived self-signed cert vs. CA+leaf? (CA+leaf is friendlier for keychain rotation.)
