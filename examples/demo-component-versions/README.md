# Example: Demo Component Versions

Pin components to specific versions for reproducible deployments.

Learn more about [Vendoring](https://atmos.tools/core-concepts/vendoring/).

## What You'll See

- [Version pinning](https://atmos.tools/core-concepts/vendoring/#versioning) with git refs
- [YAML anchors](https://atmos.tools/core-concepts/vendoring/vendor-manifest/#yaml-anchors) to DRY up vendor configs
- Multiple component versions side-by-side

## Try It

```shell
cd examples/demo-component-versions

# List vendor sources
atmos vendor list

# Pull specific versions
atmos vendor pull

# See versioned components in components/terraform/
ls components/terraform/
```

## Key Files

| File | Purpose |
|------|---------|
| `vendor.yaml` | Vendor manifest with version pinning and YAML anchors |
| `components/terraform/*/` | Versioned components (after vendor pull) |
