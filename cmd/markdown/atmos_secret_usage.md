- List declared secrets and their status

```
$ atmos secret list --stack=prod --component=api
```

- Set a declared secret's value

```
$ atmos secret set DATADOG_API_KEY --stack=prod --component=api
```

- Get a declared secret's value (masked unless ATMOS_MASK=false)

```
$ atmos secret get DATADOG_API_KEY --stack=prod --component=api
```

- Validate that all required secrets are initialized (for CI)

```
$ atmos secret validate --stack=prod --component=api
```
