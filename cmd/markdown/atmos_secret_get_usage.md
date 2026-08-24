- Get a secret value (masked in display unless masking is disabled)

```
$ atmos secret get DATADOG_API_KEY --stack=prod --component=api
```

- Reveal the real value (disables masking via env var)

```
$ ATMOS_MASK=false atmos secret get DATADOG_API_KEY --stack=prod --component=api
```

- Extract a nested value from a structured (JSON) secret

```
$ atmos secret get DATABASE_CONFIG --path=".credentials.password" --stack=prod --component=api
```

- Output as JSON or env format

```
$ atmos secret get DATADOG_API_KEY --format=json --stack=prod --component=api
```
