# License Attribution Check Action

A composite GitHub Action that checks Go dependencies for license compliance and ensures proper attribution for dependencies that require it.

## Features

- ✅ Scans all Go dependencies using `go-licenses`
- ✅ Identifies dependencies requiring attribution (Apache-2.0, BSD variants)
- ✅ Checks for disallowed license types
- ✅ Verifies NOTICE file exists when required
- ✅ **Detects missing attributions** and provides exact text to add
- ✅ Generates detailed license reports as artifacts
- ✅ Posts summary comments on pull requests with actionable instructions

## Usage

### In a Workflow

```yaml
- name: Run license attribution check
  uses: ./.github/actions/license-check
  with:
    go-version: '1.24'
    fail-on-unknown: 'true'
    create-report: 'true'
```

### Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `go-version` | Go version to use | No | `1.24` |
| `fail-on-unknown` | Fail if unknown licenses are detected | No | `true` |
| `create-report` | Generate detailed license report as artifact | No | `true` |

### Outputs

| Output | Description |
|--------|-------------|
| `licenses-requiring-attribution` | Number of dependencies requiring attribution |
| `unknown-licenses` | Number of dependencies with unknown licenses |
| `missing-attributions` | Number of dependencies missing from NOTICE file |
| `notice-exists` | Whether NOTICE file exists (true/false) |

## What It Checks

### License Types Requiring Attribution

The action checks for dependencies with the following licenses that require attribution:

- **Apache-2.0**: Requires NOTICE file with copyright notices
- **BSD-3-Clause**: Requires copyright notice in documentation
- **BSD-2-Clause**: Requires copyright notice in documentation
- **BSD-2-Clause-FreeBSD**: Requires copyright notice

### Disallowed License Types

The action fails if it finds dependencies with:

- `forbidden` - Licenses explicitly prohibited
- `restricted` - Licenses with restrictive terms
- `reciprocal` - Copyleft licenses (GPL, LGPL, etc.)

## Reports Generated

When `create-report: 'true'`, the action generates these artifacts:

1. **license-report.csv** - Clean CSV with all dependencies and their licenses
2. **license-summary.md** - Markdown summary of license distribution and attribution requirements
3. **license-report-full.log** - Full output from go-licenses including warnings

Artifacts are retained for 90 days.

## NOTICE File Requirement

The action **fails** if:
- Dependencies requiring attribution exist (Apache-2.0 or BSD licenses)
- AND no `NOTICE` file is found in the repository root

### Missing Attribution Detection

If the NOTICE file exists but is **incomplete**, the action will:
- ✅ Continue to pass (doesn't fail the build)
- ⚠️ Add a warning to the workflow summary
- 📝 Include the **exact text to add** in the PR comment
- 🎯 List missing dependencies with their license URLs

**Example output when attributions are missing:**

```markdown
## ⚠️ Missing Attributions (3)

The following dependencies require attribution but are missing from the NOTICE file:

\`\`\`
  - cel.dev/expr
    License: Apache-2.0
    URL: https://github.com/google/cel-spec/blob/v0.24.0/LICENSE

  - github.com/example/package
    License: BSD-3-Clause
    URL: https://github.com/example/package/blob/v1.0.0/LICENSE
\`\`\`

**Action Required:** Add the above entries to the appropriate section in the NOTICE file.

Or run: `./scripts/generate-notice.sh` to regenerate the entire NOTICE file.
```

This makes it easy to copy-paste the missing attributions directly into your NOTICE file.

## Example Workflow

See [`.github/workflows/license-check.yml`](../../workflows/license-check.yml) for a complete example that:

- Runs on PR changes to `go.mod`, `go.sum`, or `NOTICE`
- Posts license summary as PR comment
- Runs weekly to catch new license issues
- Uploads detailed reports as artifacts

## Troubleshooting

### "Unknown licenses detected"

Some dependencies may have non-standard LICENSE file names. Check the full log to identify them and verify they have valid licenses.

Known cases:
- `github.com/xi2/xz` - Public domain (safe to ignore)
- `inet.af/netaddr` - BSD-3-Clause (network lookup may fail)

### "NOTICE file is missing"

Create a `NOTICE` file in the repository root containing copyright notices for all Apache-2.0 and BSD-licensed dependencies.

### Action fails on `go-licenses install`

Ensure Go version matches your project's requirements. Update the `go-version` input if needed.

## Dependencies

- [go-licenses](https://github.com/google/go-licenses) - Google's tool for working with Go dependency licenses
- [actions/setup-go](https://github.com/actions/setup-go) - GitHub's Go setup action
- [actions/upload-artifact](https://github.com/actions/upload-artifact) - Artifact upload action
