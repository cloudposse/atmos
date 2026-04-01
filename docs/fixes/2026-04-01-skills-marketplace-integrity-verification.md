# Skills Marketplace: No Integrity Verification on Downloaded Skills (Finding 7)

**Date:** 2026-04-01
**Finding:** 7 — Supply-Chain Risk: Skills Marketplace Installs Unsigned Code Without Integrity Verification
**CWE:** CWE-494 — Download of Code Without Integrity Check
**Severity:** High
**Fixed by:** PR #2273

---

## Symptom

The `atmos skills install` command cloned a Git repository, validated its `SKILL.md`
metadata, and installed the files into `~/.atmos/skills/` — but it never recorded any
fingerprint of those files. Every subsequent `atmos ai chat` session re-read `SKILL.md`
and all referenced files fresh from disk with no comparison against the state that was
reviewed and accepted at install time.

This meant that modifying any file inside an installed skill directory was completely
undetectable by Atmos. There was no warning, no error, and no way for a user to know
whether a skill had been changed after installation.

---

## Root Cause

`LoadInstalledSkills` in `pkg/ai/skills/marketplace/installer.go` walked the local
registry, read each skill's `SKILL.md`, and loaded `SystemPrompt` and `AllowedTools`
directly from the on-disk content without any integrity check:

```go
// Before the fix — no integrity verification
skillMDPath := filepath.Join(installed.Path, skillFileName)
metadata, err := ParseSkillMetadata(skillMDPath)
// ... loaded directly into the skills.Registry
```

`InstalledSkill` in `local_registry.go` stored only `Name`, `Version`, `Source`, and
`Path` — no cryptographic fingerprint of the installed files.

---

## Attack Scenarios

### Scenario A — Post-install local tamper

A malicious process (or a compromised CI/CD artifact) modifies `SKILL.md` in
`~/.atmos/skills/github.com/owner/repo/` after the user has already reviewed and
accepted the skill. On the next `atmos ai chat` session the injected `SystemPrompt`
or expanded `AllowedTools` list is silently loaded.

### Scenario B — Malicious update via `--force`

A skill author publishes a legitimate v1.0.0. A user installs it and trusts it. The
author later deploys a malicious v1.0.1 to the same repository. If the user runs
`atmos skills install --force owner/repo` to "update", they see the confirmation prompt —
but if they skip it with `--skip-confirm` (common in CI/CD pipelines) there is no hash
to compare against.

### Scenario C — Path traversal / symlink injection (defence in depth)

Without a content hash, a post-install symlink planted inside the skill directory could
cause Atmos to read arbitrary files on the filesystem. A content hash computed over real
files at install time provides an independent defence layer.

---

## Fix

### 1. `ErrSkillHashMismatch` sentinel (`errors.go`)

```go
ErrSkillHashMismatch = errors.New("skill content hash mismatch — skill files may have been tampered with")
```

### 2. `ContentHash` field in `InstalledSkill` (`local_registry.go`)

```go
ContentHash string `json:"content_hash"` // SHA-256 of all skill files at install time.
```

The field is optional (`omitempty`-compatible) for backward compatibility: skills
installed before this fix have an empty `ContentHash` and continue to load without
verification (legacy path).

### 3. `computeSkillHash` and `verifySkillHash` (`installer.go`)

`computeSkillHash(skillDir string) (string, error)`:

* Walks all files under `skillDir` in deterministic lexicographic order.
* Skips the `.git` directory entirely.
* Feeds `\x00<forward-slash-relative-path>\x00<file-content>` into a `sha256.New()` hasher
  for each file — the path prefix prevents a content-only collision between files whose
  bytes happen to be identical.
* Returns the lowercase hex digest (64 characters).
* Path normalisation to forward slashes makes the hash platform-independent: the same
  skill produces the same hash on Windows and Linux.

`verifySkillHash(skillDir, expected string) error`:
Recomputes the hash and returns `ErrSkillHashMismatch` when the digests differ.

### 4. Hash stored at install time

Both install paths (single-skill and multi-skill package) now:

1. Compute the hash from the downloaded temporary directory **before** files are moved
   or copied to `~/.atmos/skills/`.
2. Persist the hash in `registry.json` via the `ContentHash` field.

The confirmation prompt also prints the digest:

```
Skill: My Skill
Author: Example Author
Version: 1.0.0
Repository: https://github.com/owner/my-skill
SHA-256: 3a7bd3e2360a3d29eea436fcfb7e44c735d117c4...

Do you want to install this skill? [y/N]
```

This allows users to verify the hash out-of-band against their own checkout of the
repository.

### 5. Hash verified at load time (`LoadInstalledSkills`)

Before any skill is registered into the active `skills.Registry`:

```go
if installed.ContentHash != "" {
    if err := verifySkillHash(installed.Path, installed.ContentHash); err != nil {
        log.Warnf("Integrity check failed for skill %q: %v — skipping", installed.Name, err)
        continue
    }
}
```

A tampered skill emits a warning and is skipped; it is never loaded into the LLM context.

---

## Files Changed

| File | Change |
|---|---|
| `pkg/ai/skills/marketplace/errors.go` | Add `ErrSkillHashMismatch` |
| `pkg/ai/skills/marketplace/local_registry.go` | Add `ContentHash` to `InstalledSkill` |
| `pkg/ai/skills/marketplace/installer.go` | `computeSkillHash`, `verifySkillHash`; thread hash through install and load paths; display hash in confirmation prompt |
| `pkg/ai/skills/marketplace/installer_test.go` | 9 new tests; 2 existing tests updated for changed `confirmInstallation` signature |

---

## Tests Added

| Test | What it verifies |
|---|---|
| `TestComputeSkillHash_Success` | Hash is produced and is a 64-char hex string |
| `TestComputeSkillHash_Deterministic` | Same directory → same hash on repeated calls |
| `TestComputeSkillHash_DifferentContentDifferentHash` | Different content → different hash |
| `TestComputeSkillHash_SkipsGitDir` | Adding a `.git` directory does not change the hash |
| `TestVerifySkillHash_Success` | Correct hash passes verification |
| `TestVerifySkillHash_Mismatch` | Wrong hash returns `ErrSkillHashMismatch` |
| `TestInstall_ContentHashStoredInRegistry` | After `Install`, registry entry has non-empty 64-char `ContentHash` |
| `TestLoadInstalledSkills_TamperedSkillSkipped` | Modifying `SKILL.md` after install causes the skill to be skipped on load |
| `TestLoadInstalledSkills_LegacySkillWithoutHash` | Skills with empty `ContentHash` (legacy) still load without error |

---

## Backward Compatibility

* `registry.json` gains a new `content_hash` key per skill entry.
  Older Atmos versions that do not know about this field will silently ignore it (JSON
  `omitempty` / unknown-field tolerance).
* Skills already installed before this fix have `content_hash: ""`. The load path treats
  an empty hash as "no verification required", so existing installations continue to work.
* Users who want integrity protection for already-installed skills should reinstall them:
  `atmos skills install --force owner/repo`.

---

## Related

- PR #2273: sec: add SHA-256 content-hash integrity verification to skills marketplace installer
- `pkg/ai/skills/marketplace/installer.go`: `computeSkillHash`, `verifySkillHash`, `LoadInstalledSkills`
- `pkg/ai/skills/marketplace/local_registry.go`: `InstalledSkill.ContentHash`
- `pkg/ai/skills/marketplace/errors.go`: `ErrSkillHashMismatch`
