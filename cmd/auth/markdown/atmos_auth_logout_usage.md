```shell
# Logout from specific identity (preserves keychain)
$ atmos auth logout dev-admin

# Permanently delete keychain credentials (interactive)
$ atmos auth logout dev-admin --keychain

# Logout from all identities
$ atmos auth logout --all

# Logout from provider
$ atmos auth logout --provider my-cloud-provider

# Preview without deleting
$ atmos auth logout dev-admin --dry-run

# Non-interactive logout for CI/CD (skips confirmation prompts)
$ atmos auth logout dev-admin --force
```
