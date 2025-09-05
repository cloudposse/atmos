– Encrypt and decrypt files with Ansible Vault

```
$ atmos ansible vault <component-name> -s <stack-name> encrypt <file>
```

– Decrypt a vault file

```
$ atmos ansible vault <component-name> -s <stack-name> decrypt <file>
```

– Create a new encrypted file

```
$ atmos ansible vault <component-name> -s <stack-name> create <file>
```