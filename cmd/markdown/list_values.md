– List all values for a component
```
 $ atmos list values <component>
```
– Show only variables for a component
```
 $ atmos list values <component> --query .vars
```
– Include abstract components
```
 $ atmos list values <component> --abstract
```
– Limit number of columns
```
 $ atmos list values <component> --max-columns 5
```
– Output in different formats
```
 $ atmos list values <component> --format json
 $ atmos list values <component> --format yaml
 $ atmos list values <component> --format csv
```
– Filter stacks using patterns
```
 $ atmos list values <component> --stack '*-dev-*'
 $ atmos list values <component> --stack 'prod-*'
```
