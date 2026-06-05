- Import secrets from a file, skipping (and warning about) undeclared keys

```
$ atmos secret import .env --stack=prod --component=api
```

- Preview what would be imported without writing

```
$ atmos secret import .env --stack=prod --component=api --dry-run
```

- Import from stdin

```
$ cat .env | atmos secret import - --stack=prod --component=api
```
