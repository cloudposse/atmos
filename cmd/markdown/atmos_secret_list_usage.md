- List declared secrets and their initialization status

```
$ atmos secret list --stack=prod --component=api
```

- Show declaration descriptions

```
$ atmos secret list --stack=prod --component=api --verbose
```

- Output for pipelines or automation

```
$ atmos secret list --stack=prod --component=api --format=json
$ atmos secret list --stack=prod --component=api --format=csv
```
