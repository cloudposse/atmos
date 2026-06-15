# ⚠️ Test fixture — never commit private keys in a real project

This directory commits a SOPS **age private key** (`keys.txt`) next to the encrypted
file it unlocks (`dev.enc.yaml`) **only** so this test fixture is self-contained and
runs in-process with no setup. The key is a throwaway generated for this fixture.

**In a real project, the private key must NEVER be committed.** Distribute it out of
band and load it via `SOPS_AGE_KEY_FILE` or your OS keychain; commit only the
encrypted `*.enc.yaml` file. See the
[Secrets configuration guide](https://atmos.tools/cli/configuration/secrets).
