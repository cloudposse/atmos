- Delete a secret's value from its backend (prompts for confirmation)

```
$ atmos secret delete DATADOG_API_KEY --stack=prod --component=api
```

- Delete without confirmation (alias: rm)

```
$ atmos secret rm DATADOG_API_KEY --stack=prod --component=api --force
```
