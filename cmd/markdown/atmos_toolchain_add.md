- Add a tool with version
```
 $ atmos toolchain add <tool@version>
```

- Add a tool (defaults to latest version)
```
 $ atmos toolchain add <tool>
```

- Add multiple tools at once
```
 $ atmos toolchain add <tool[@version]> <tool[@version]>...
```

- Use a custom tool-versions file
```
 $ atmos toolchain add --tool-versions <path/to/.tool-versions> <tool[@version]>
```
