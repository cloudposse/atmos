- Push secret values from a local .env file (every key must be declared)

```
$ atmos secret push --stack=dev --component=api --input=.env
```

- Push from JSON

```
$ atmos secret push --stack=dev --component=api --format=json --input=secrets.json
```
