---
slug: interactive-identity-selection
title: "Interactive Identity Selection for Auth Login"
authors: [osterman]
tags: [enhancement, auth]
---

Running `atmos auth login` without specifying an identity is now more user-friendly. When no `--identity` flag is provided, Atmos presents an interactive selector to choose from your configured identities.

<!--truncate-->

## What Changed

The `atmos auth login` command now provides an interactive identity selector when no identity is specified on the command line. This makes it easier to authenticate without remembering exact identity names.

## How It Works

When you run `atmos auth login` without the `--identity` flag:

**Interactive Mode:**
- If exactly one default identity is configured → uses it automatically
- If no default identity is configured → shows an interactive selector with all available identities
- If multiple default identities are configured → shows an interactive selector with those defaults

**CI/CD Mode (non-interactive):**
- Returns an error if no default identity is found
- Requires using the `--identity` flag or `ATMOS_IDENTITY` environment variable

## Example

```bash
# No identity specified - shows interactive selector
$ atmos auth login

# Use arrow keys to navigate and Enter to select:
> dev-admin
  prod-readonly
  staging-deploy
```

## Migration

No changes required! This enhancement is fully backward compatible:

- Existing commands with `--identity` flag work exactly as before
- Default identity configuration continues to work
- CI/CD pipelines are unaffected (they should already specify identity explicitly)

## Notes

The interactive selector uses arrow keys for navigation and Enter to confirm selection. It's available in terminal environments and provides a better experience when working with multiple identities.

For more details, see the [auth login documentation](/cli/commands/auth/login).
