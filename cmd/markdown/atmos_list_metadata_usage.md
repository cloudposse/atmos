– List all metadata
```
 $ atmos list metadata
```

– List metadata for specific stacks
```
 $ atmos list metadata --stack '*-dev-*'
```

– List specific metadata fields
```
 $ atmos list metadata --query .metadata.component
 $ atmos list metadata --query .metadata.type
```

– Output in different formats
```
 $ atmos list metadata --format json
 $ atmos list metadata --format yaml
 $ atmos list metadata --format csv
 $ atmos list metadata --format tsv
```

– Filter by stack and specific metadata
```
 $ atmos list metadata --stack '*-ue2-*' --query .metadata.version
```
