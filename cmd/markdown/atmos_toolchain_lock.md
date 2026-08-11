
- Lock every tool in .tool-versions
```
 $ atmos toolchain lock
```

- Lock a specific tool
```
 $ atmos toolchain lock terraform
```

- Lock multiple tools with a custom concurrency limit
```
 $ atmos toolchain lock terraform kubectl --max-concurrency 2
```
