- Delete a secret's value from its backend (prompts for confirmation)

```
$ atmos secret delete DATADOG_API_KEY --stack=prod --component=api
```

- Delete without confirmation (aliases: rm, unset)

```
$ atmos secret rm DATADOG_API_KEY --stack=prod --component=api --force
$ atmos secret unset DATADOG_API_KEY --stack=prod --component=api --force
```
