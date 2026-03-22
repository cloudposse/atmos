# Native CI Integration - Describe Affected Matrix Format

> Related: [Overview](../overview.md) | [CI Outputs](../providers/github/ci-outputs.md)

## FR-8: Describe Affected Matrix Format (IMPLEMENTED)

**Requirement**: Output affected components in GitHub Actions matrix format.

**Implementation**: `cmd/describe_affected.go` adds `--format=matrix` flag. `internal/exec/describe_affected.go` implements `MatrixOutput`/`MatrixEntry` structs, `convertAffectedToMatrix()`, and `writeMatrixOutput()` with `--output-file` support for `$GITHUB_OUTPUT` (`matrix=<json>` format).

**Behavior**:
- `atmos describe affected --format=matrix` outputs JSON matrix to stdout
- `--output-file=$GITHUB_OUTPUT` writes `matrix=<json>` for downstream jobs
- Format directly consumable by GitHub Actions `matrix` strategy
- Include component and stack for each affected item

**Usage**:
```bash
# Output matrix JSON to stdout
atmos describe affected --format=matrix

# Write to $GITHUB_OUTPUT for use in subsequent jobs
atmos describe affected --format=matrix --output-file="$GITHUB_OUTPUT"
```

**Output Format** (fixed schema — `component`, `stack`, `component_path`, `component_type` fields):
```json
{"include":[{"component":"vpc","stack":"dev","component_path":"components/terraform/vpc","component_type":"terraform"},{"component":"eks","stack":"dev","component_path":"components/terraform/eks","component_type":"terraform"}]}
```

> **Design Decision**: The matrix includes `component`, `stack`, `component_path`, and `component_type` fields. The `component_path` and `component_type` fields are included for convenience in downstream workflow steps.

## GitHub Actions Integration

Output (stdout):

```json
{"include":[{"component":"vpc","stack":"plat-ue2-dev","component_path":"components/terraform/vpc","component_type":"terraform"},{"component":"eks","stack":"plat-ue2-dev","component_path":"components/terraform/eks","component_type":"terraform"}]}
```

Output ($GITHUB_OUTPUT — two variables written):

```
matrix={"include":[{"component":"vpc","stack":"plat-ue2-dev","component_path":"components/terraform/vpc","component_type":"terraform"},{"component":"eks","stack":"plat-ue2-dev","component_path":"components/terraform/eks","component_type":"terraform"}]}
affected_count=2
```

This format is directly consumable by GitHub Actions matrix strategy:

```yaml
jobs:
  affected:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.affected.outputs.matrix }}
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
