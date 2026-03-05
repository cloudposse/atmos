# Native CI Integration - Describe Affected Matrix Format

> Related: [Overview](./overview.md) | [CI Outputs](./ci-outputs.md)

## FR-8: Describe Affected Matrix Format

**Requirement**: Output affected components in GitHub Actions matrix format.

**Behavior**:
- `atmos describe affected --format=matrix` outputs JSON matrix to stdout
- `--output-file=$GITHUB_OUTPUT` writes `affected=<json>` for downstream jobs
- Format directly consumable by GitHub Actions `matrix` strategy
- Include component and stack for each affected item

**Usage**:
```bash
# Output matrix JSON to stdout
atmos describe affected --format=matrix

# Write to $GITHUB_OUTPUT for use in subsequent jobs
atmos describe affected --format=matrix --output-file="$GITHUB_OUTPUT"
```

**Output Format**:
```json
{"include":[{"component":"vpc","stack":"dev"},{"component":"eks","stack":"dev"}]}
```

## GitHub Actions Integration

Output (stdout):

```json
{"include":[{"component":"vpc","stack":"plat-ue2-dev"},{"component":"eks","stack":"plat-ue2-dev"}]}
```

Output ($GITHUB_OUTPUT):

```
affected={"include":[{"component":"vpc","stack":"plat-ue2-dev"},{"component":"eks","stack":"plat-ue2-dev"}]}
```

This format is directly consumable by GitHub Actions matrix strategy:

```yaml
jobs:
  affected:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.affected.outputs.affected }}
    steps:
      - uses: actions/checkout@v4
      - name: Get affected components
        id: affected
        run: atmos describe affected --format=matrix --output-file="$GITHUB_OUTPUT"

  plan:
    needs: affected
    strategy:
      matrix: ${{ fromJson(needs.affected.outputs.matrix) }}
```

## Describe Affected Flags

| Flag | Description |
|------|-------------|
| `--format=matrix` | Output GitHub Actions matrix format |
| `--output-file` | Write output to file in key=value format (for `$GITHUB_OUTPUT`) |
