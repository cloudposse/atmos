# ⚠️ Demo secrets — never do this in a real project

This directory commits a SOPS **age private key** (`keys.txt`) alongside the
encrypted file it unlocks (`dev.enc.yaml`). **This is for demonstration only**, so
the example is self-contained and runs with no setup or cloud credentials.

**In a real project, the private key must NEVER be committed.** Anyone with both
the key and the encrypted file can read every secret — committing them together
defeats the encryption entirely.

## What to do instead

- **Keep the private key out of the repo.** Distribute it out of band (a password
  manager, your secrets backend, onboarding docs) — never in version control.
- **Load it from outside the repo** via `SOPS_AGE_KEY_FILE` pointing at a path in
  your home directory, or from your OS keychain. The stack's `spec.age_key_file`
  also supports `~` and `$ENV` expansion for this.
- **Commit only the encrypted file** (`*.enc.yaml`). That is safe to commit and is
  the whole point of SOPS.
- **Gitignore decrypted scratch files** (`*.dec.yaml` — already ignored here).

See [`../README.md`](../README.md) and the
[Secrets configuration guide](https://atmos.tools/cli/configuration/secrets) for how
to keep the age key in your OS keychain instead of a file.
