- Interactive mode (select template and target)

```
$ atmos init
```

- Initialize with simple template in current directory

```
$ atmos init simple .
```

- Initialize an emulator-ready AWS application repository

```
$ atmos init aws/app ./my-app --set project_name=my-app
```

- Initialize a landing zone scaffold without creating the initial git commit

```
$ atmos init gcp/landing-zone ./foundation --no-git --set project_name=my-foundation
```

- Initialize from a template repository at a specific ref

```
$ atmos init git::https://github.com/example/scaffolds.git//aws-app ./my-app --ref v1.2.3
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
