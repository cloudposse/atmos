- Logout from default identity (preserves keychain credentials)

```shell
atmos auth logout
```

- Logout from specific identity (preserves keychain credentials)

```shell
atmos auth logout dev-admin
```

- Logout and permanently delete keychain credentials (interactive confirmation)

```shell
atmos auth logout dev-admin --keychain
```

- Logout and permanently delete keychain credentials (bypass confirmation for CI/CD)

```shell
atmos auth logout dev-admin --keychain --force
```

- Logout from all identities (preserves keychain credentials)

```shell
atmos auth logout --all
```

- Logout from all identities and delete all keychain credentials

```shell
atmos auth logout --all --keychain --force
```

- Logout from specific provider (preserves keychain credentials)

```shell
atmos auth logout --provider my-cloud-provider
```

- Logout from specific provider and delete keychain credentials

```shell
atmos auth logout --provider my-cloud-provider --keychain --force
```

- Preview what would be removed without actually deleting

```shell
atmos auth logout dev-admin --dry-run
```

- Preview keychain deletion without actually deleting

```shell
atmos auth logout dev-admin --keychain --dry-run
```
