- Print credentials for an identity

```shell
atmos aws credential-process --identity=app-sandbox-1
```

- Use it from `~/.aws/config`

```shell
[profile app-sandbox-1]
credential_process = atmos --chdir=/path/to/infrastructure aws credential-process --identity=app-sandbox-1
```

- Require at least 30 minutes of remaining validity

```shell
atmos aws credential-process --identity=app-sandbox-1 --min-validity=30m
```
