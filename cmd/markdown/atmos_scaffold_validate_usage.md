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
✓ Scaffold configuration is valid
✓ All required fields are defined
✓ Field types are correct
✓ Default values match field types
```