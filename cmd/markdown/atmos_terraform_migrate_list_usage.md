- List tfmigrate context for every component in every stack

```shell
  $ atmos terraform migrate list
```

- List for a single component

```shell
  $ atmos terraform migrate list vpc -s prod-ue2
```

- Choose specific columns to display

```shell
  $ atmos terraform migrate list --columns component,stack,mode,history_key
```

- Output as JSON

```shell
  $ atmos terraform migrate list -f json
```

- Sort by a specific column

```shell
  $ atmos terraform migrate list --sort component:asc
```
