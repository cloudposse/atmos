– List all settings
```
 $ atmos list settings
```

– List settings for specific stacks
```
 $ atmos list settings --stack '*-dev-*'
```

– List specific settings using query
```
 $ atmos list settings --query .settings.templates
 $ atmos list settings --query .settings.validation
```

– Output in different formats
```
 $ atmos list settings --format json
 $ atmos list settings --format yaml
 $ atmos list settings --format csv
 $ atmos list settings --format tsv
```

– Filter by stack and specific settings
```
 $ atmos list settings --stack '*-ue2-*' --query .settings.templates.gomplate
```
