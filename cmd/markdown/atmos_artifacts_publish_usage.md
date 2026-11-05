- Publish a directory

```
$ atmos artifacts publish ./dist --repository=backups --name=builds/app --version=build-123
```

- Replace an existing reference with different content

```
$ atmos artifacts publish ./dist --repository=backups --name=builds/app --version=build-123 --force
```

- Plan a publication without writing anything

```
$ atmos artifacts publish ./dist --repository=backups --name=builds/app --version=build-124 --dry-run
```

- Attach labels and documents

```
$ atmos artifacts publish ./dist --repository=backups --name=builds/app --version=build-125 \
    --labels=team=platform,purpose=backup --readme=README.md --changelog=CHANGELOG.md
```
