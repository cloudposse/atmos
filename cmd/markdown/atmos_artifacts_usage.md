- Publish a directory as a versioned artifact

```
$ atmos artifacts publish ./dist --repository=backups --name=builds/app --version=build-123
```

- Pull, copy, inspect, and list

```
$ atmos artifacts pull builds/app:build-123 --repository=backups --output=./restore
$ atmos artifacts copy builds/app:build-123 --from=backups --to=shared
$ atmos artifacts inspect builds/app:build-123 --repository=backups
$ atmos artifacts list --repository=backups
```

- Sign and verify

```
$ atmos artifacts sign 'builds/app@sha256:<digest>' --repository=backups
$ atmos artifacts verify builds/app:build-123 --repository=backups
```
