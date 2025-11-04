- Execute a command in a devcontainer

```
$ atmos devcontainer exec geodesic -- ls -la
```

- Execute in a specific instance

```
$ atmos devcontainer exec terraform --instance my-instance -- terraform version
```

- Run commands that output sensitive data (automatically masked)

```
$ atmos devcontainer exec geodesic -- env | grep AWS
```

- Disable masking if needed

```
$ atmos devcontainer exec geodesic --mask=false -- cat ~/.aws/config
```

**Automatic Masking**: Output from `exec` commands is automatically masked based on patterns configured in `atmos.yaml`. This protects sensitive data like AWS keys, GitHub tokens, and other secrets from being displayed in plain text. Unlike interactive shells (`atmos devcontainer shell`), which cannot be masked due to TTY limitations, `exec` runs commands in non-interactive mode where masking works reliably.
