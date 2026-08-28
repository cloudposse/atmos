• Validate scaffold configuration in current directory

```
$ atmos scaffold validate
```

• Validate specific scaffold.yaml file

```
$ atmos scaffold validate ./templates/myapp/scaffold.yaml
```

• Validate scaffold in specific directory

```
$ atmos scaffold validate ./templates/react-app
```

• Example successful validation output

```
$ atmos scaffold validate ./templates/simple
ℹ Validating ./templates/simple/scaffold.yaml
✓ ./templates/simple/scaffold.yaml: valid

Validation Summary:
✓ Valid files: 1
```
