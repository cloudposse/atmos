– Execute ansible commands for configuration management

```
$ atmos ansible playbook <component-name> -s <stack-name>
```

```
$ atmos ansible inventory <component-name> -s <stack-name> --list
```

```
$ atmos ansible vault <component-name> -s <stack-name> encrypt <file>
```