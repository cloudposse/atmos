- Generate from a remote scaffold template

```
$ atmos scaffold generate https://github.com/user/scaffold-template
```

- Generate from a template repository at a specific ref

```
$ atmos scaffold generate git::https://github.com/example/scaffolds.git//component ./output --ref v1.2.3
```

- Generate to a specific target directory

```
$ atmos scaffold generate ./local-template ./output-directory
```

- Generate a project and initialize a git repository

```
$ atmos scaffold generate aws/app ./my-app --git --set project_name=my-app
```

- Force overwrite existing files

```
$ atmos scaffold generate ./template ./output --force
```

- Dry run to preview changes

```
$ atmos scaffold generate ./template ./output --dry-run
```

- Set template variables

```
$ atmos scaffold generate ./template ./output --set name=myapp --set version=1.0.0
```
