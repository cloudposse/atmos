- Generate from a remote scaffold template

```
$ atmos scaffold generate https://github.com/user/scaffold-template
```

- Generate to a specific target directory

```
$ atmos scaffold generate ./local-template ./output-directory
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