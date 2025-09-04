– Execute an Ansible playbook

```
$ atmos ansible playbook <component-name> -s <stack-name>
```

– Execute with custom playbook

```
$ atmos ansible playbook <component-name> -s <stack-name> --playbook custom.yml
```

– Execute with custom inventory

```
$ atmos ansible playbook <component-name> -s <stack-name> --inventory inventory.yml
```