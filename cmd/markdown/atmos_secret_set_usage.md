- Set a secret interactively (prompts for the value, input hidden)

```
$ atmos secret set DATADOG_API_KEY --stack=prod --component=api
```

- Set a secret non-interactively (for CI)

```
$ atmos secret set DATADOG_API_KEY=dd-abc123 --stack=prod --component=api
```

- Set a large value (e.g. a private key) from stdin

```
$ cat private-key.pem | atmos secret set GITHUB_APP_KEY --stdin --stack=prod --component=api
```

- Overwrite an existing value without confirmation (alias: add)

```
$ atmos secret add DATADOG_API_KEY=dd-abc123 --stack=prod --component=api --force
```
