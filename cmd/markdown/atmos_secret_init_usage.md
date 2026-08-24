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

- Initialize declared secrets across every stack

```
$ atmos secret init --all
```

- Initialize declared values from a dotenv file (comments are supported)

```
$ atmos secret init --input .env.local --stack=prod --component=api
```

- Initialize from redirected stdin

```
$ atmos secret init --stack=prod --component=api < .env.local
```
