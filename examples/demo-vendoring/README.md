# Example: Demo Vendoring

Pull Terraform modules from GitHub, S3, or OCI registries with pinned versions.

Learn more about [Vendoring](https://atmos.tools/core-concepts/vendoring/).

## What You'll See

- [Vendor manifest](https://atmos.tools/core-concepts/vendoring/vendor-manifest/) defining component sources
- Multiple [vendor sources](https://atmos.tools/core-concepts/vendoring/#sources) from GitHub
- Version pinning with `ref` parameter
- Modular vendor configs in `vendor.d/`

## Try It

```shell
cd examples/demo-vendoring

# List available vendor sources
atmos vendor list

# Pull all vendored components
atmos vendor pull

# Pull a specific component
atmos vendor pull --component weather
```

## Key Files

| File | Purpose |
|------|---------|
| `vendor.yaml` | Main vendor manifest with component sources |
| `vendor.d/` | Modular vendor configurations |
| `vendor/` | Downloaded components (after `atmos vendor pull`) |
