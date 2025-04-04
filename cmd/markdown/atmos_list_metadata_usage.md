– List all metadata
```
 $ atmos list metadata
```

– List specific metadata
```
 $ atmos list metadata --query '.component'
```

– Filter by stack pattern
```
 $ atmos list metadata --stack '*-dev-*'
```

– Output in different formats
```
 $ atmos list metadata --format json
 $ atmos list metadata --format yaml
 $ atmos list metadata --format csv
 $ atmos list metadata --format tsv
```

– Disable Go template processing
```
 $ atmos list metadata --process-templates=false
```

– Disable YAML functions processing
```
 $ atmos list metadata --process-functions=false
```

- Stack patterns support glob matching (e.g., `*-dev-*`, `prod-*`, `*-{dev,staging}-*`)
