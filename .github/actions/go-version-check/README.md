# Go Version Check Action

Detects and comments on Go version changes in pull requests.

## Usage

```yaml
- uses: ./.github/actions/go-version-check
  with:
    base-ref: ${{ github.base_ref }}
    token: ${{ secrets.GITHUB_TOKEN }}
```

## Inputs

- `base-ref` (required): Base branch reference to compare against
- `token` (required): GitHub token for API access (defaults to `github.token`)

## Outputs

- `base-version`: Go version in base branch
- `pr-version`: Go version in PR
- `changed`: Whether the Go version changed (`true`/`false`)
- `is-upgrade`: Whether this is an upgrade (`true`) or downgrade (`false`)

## Features

- ✅ Detects Go version changes by comparing `go.mod`
- ⬆️ Provides upgrade checklist for Go upgrades
- ⬇️ Warns about downgrades with rebase instructions
- 🔄 Updates existing comments instead of creating duplicates
- 📋 Links to Go release notes

## Example Output

### Upgrade
```
🚀 Go Version Change Detected

This PR changes the Go version:
- Base branch (main): 1.24.8
- This PR: 1.25.0
- Change: ⬆️ Upgrade

> [!TIP]
> ### Upgrade Checklist
> - [ ] Verify all CI workflows pass with new Go version
> - [ ] Check for new language features that could be leveraged
> - [ ] Review release notes: https://go.dev/doc/go1.25
> - [ ] Update .tool-versions if using asdf
> - [ ] Update Dockerfile Go version if applicable
```

### Downgrade
```
⚠️ Go Version Change Detected

This PR changes the Go version:
- Base branch (main): 1.25.0
- This PR: 1.24.8
- Change: ⬇️ Downgrade

> [!WARNING]
> ### Downgrade Detected
> Downgrading Go version may indicate:
> - This PR was based on an outdated branch
> - Consider rebasing on latest main
> - Verify this change is intentional
>
> **If unintentional, please:**
> 1. Rebase on latest main
> 2. Run `go mod tidy`
> 3. Push updated changes
```
