# GitHub Issue: Migrate remaining archived dependencies

**Title**: Migrate remaining archived dependencies

**Labels**: enhancement, dependencies, tech-debt

---

## Summary

Audit of Atmos dependencies found 14 archived (unmaintained) repositories. One migration has been completed, three high-priority items remain.

**Status**: 1 of 4 high-impact migrations complete ✅

---

## Completed Migrations

### ✅ golang/mock → go.uber.org/mock v0.6.0
- Updated all imports and mockgen directives
- Regenerated all mock files
- Tests passing, build successful

---

## Remaining High-Priority Migrations

### 1. pkg/errors → stdlib errors (High Impact)
- **Status**: ARCHIVED (December 1, 2021)
- **Current**: `github.com/pkg/errors` v0.9.1
- **Target**: Go stdlib `errors` + `fmt.Errorf` (Go 1.13+)
- **Alternative**: `github.com/cockroachdb/errors` (100% compatible)
- **Note**: Migration strategy already documented in `docs/prd/error-handling-strategy.md`
- **Action**: Continue gradual migration per existing strategy

### 2. mitchellh/mapstructure v1 → v2 (High Impact)
- **Status**: ARCHIVED
- **Current**: `github.com/mitchellh/mapstructure` v1.5.0 (heavily used)
- **Target**: `github.com/go-viper/mapstructure/v2` v2.4.0 (blessed fork)
- **Note**: We already use v2 in `pkg/auth/hooks.go`
- **Action**:
  - Audit all v1 usages (12+ files)
  - Test compatibility with v2
  - Gradual migration following `pkg/auth/hooks.go` pattern

### 3. Verify AWS SDK v1 not directly imported (Medium Priority)
- **Status**: aws-sdk-go v1 is archived/deprecated
- **Current**: Should be using aws-sdk-go-v2 only
- **Action**:
  - Search for `github.com/aws/aws-sdk-go` imports (not v2)
  - Ensure all AWS code uses v2 SDK

---

## Low Priority: Indirect Dependencies (Monitor Only)

These are transitive dependencies that should be replaced by upstream maintainers:

- `github.com/AlecAivazis/survey` (indirect)
- `github.com/Azure/go-autorest` v14.2.0+incompatible (indirect)
- `github.com/aws/aws-sdk-go` v1.55.7 (indirect)
- `github.com/golang/snappy` v0.0.4 (indirect)
- `github.com/google/wire` v0.6.0 (indirect)
- `github.com/hexops/gotextdiff` v1.0.3 (indirect)
- `github.com/mitchellh/copystructure` v1.2.0 (indirect)
- `github.com/mitchellh/hashstructure` v2.0.2 (indirect)
- `github.com/mitchellh/reflectwalk` v1.0.2 (indirect)
- `github.com/rcrowley/go-metrics` (indirect)

**Action**: Monitor for upstream replacements, no immediate action required.

---

## Already Addressed

### ✅ mitchellh/go-homedir
- Using vendored fork at `./pkg/config/homedir` with test enhancements
- Addressed via `replace` directive in go.mod

---

## Verification Commands

```bash
# Check for pkg/errors usage
grep -r "github.com/pkg/errors" --include="*.go" .

# Check for mitchellh/mapstructure v1 usage
grep -r "github.com/mitchellh/mapstructure" --include="*.go" .

# Check for AWS SDK v1 imports
grep -r '"github.com/aws/aws-sdk-go"' --include="*.go" .
```

---

## References

- Mitchell Hashimoto Archive Announcement: https://gist.github.com/mitchellh/90029601268e59a29e64e55bab1c5bdc
- Error Handling Strategy: `docs/prd/error-handling-strategy.md`
- Atmos homedir fork: `pkg/config/homedir/README.md`

---

## Task Breakdown

- [x] Migrate golang/mock → go.uber.org/mock
- [ ] Migrate pkg/errors → stdlib
- [ ] Migrate mitchellh/mapstructure v1 → v2
- [ ] Verify AWS SDK v1 not directly imported
- [ ] Monitor indirect dependencies for upstream replacements

---

## Instructions

Create this issue manually using:
```bash
gh issue create --title "Migrate remaining archived dependencies" --body-file GITHUB_ISSUE_ARCHIVED_DEPS.md --label "enhancement" --label "dependencies" --label "tech-debt"
```

Or copy the content above and create via GitHub web UI.
