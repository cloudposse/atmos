- Provision all missing required secrets for a component (interactive prompts)

```
$ atmos secret init --stack=prod --component=api
```

- Re-prompt and overwrite already-initialized secrets

```
$ atmos secret init --stack=prod --component=api --force
```

- Preview what would be initialized without prompting or writing

```
$ atmos secret init --stack=prod --component=api --dry-run
```
