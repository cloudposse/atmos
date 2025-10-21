- Interactive mode (select template and target)

```
$ atmos init
```

- Initialize with simple template in current directory

```
$ atmos init simple .
```

- Initialize with atmos template in new directory

```
$ atmos init atmos ./my-project
```

- Force overwrite existing files

```
$ atmos init simple ./my-project --force
```

- Set template values directly

```
$ atmos init simple ./my-project --set project_name=myapp --set namespace=dev
```