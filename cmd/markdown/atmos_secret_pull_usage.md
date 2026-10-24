- Pull declared secrets to a local .env file for development

```
$ atmos secret pull --stack=dev --component=api --output=.env
```

- Pull as JSON

```
$ atmos secret pull --stack=dev --component=api --format=json --output=secrets.json
```
