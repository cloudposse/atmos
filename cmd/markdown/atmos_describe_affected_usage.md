- Specify Filesystem path to the already cloned target repository with which to compare the current branch

```
 $ atmos describe affected --repo-path <path_to_already_cloned_repo>
```

- Specify Git reference with which to compare the current branch

```
 $ atmos describe affected --ref refs/heads/main
```

- Specify Git commit SHA with which to compare the current branch

```
 $ atmos describe affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073
```

- Specify the file to write the result

```
 $ atmos describe affected --ref refs/tags/v1.75.0 --file affected.json
```

- Specify the output format for the file

```
 $ atmos describe affected --format=json|yaml
```

- Print more detailed output when cloning and checking out the Git repository
```
 $ atmos describe affected --verbose=true
```

- Specify Path to PEM-encoded private key to clone private repos using SSH

```
 $ atmos describe affected --ssh-key <path_to_ssh_key>
```

- Specify Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block

```
 $ atmos describe affected --ssh-key <path_to_ssh_key> --ssh-key-password <password>
```

- Include the Spacelift admin stack of any stack that is affected by config changes

```
 $ atmos describe affected --include-spacelift-admin-stacks=true
```

- Include the dependent components and stacks

```
 $ atmos describe affected --include-dependents=true
```

- Include the `settings` section for each affected component

```
 $ atmos describe affected --include-settings=true
```

- Upload the affected components and stacks to a specified HTTP endpoint

```bash
 $ atmos describe affected --upload
```

- Clone the target reference with which to compare the current branch

```
 $ atmos describe affected --clone-target-ref=true
```

- Enable/disable Go template processing in Atmos stack manifests when executing the command

```
 $ atmos describe affected --process-templates=false
```

- Enable/disable YAML functions processing in Atmos stack manifests when executing the command

```
 $ atmos describe affected --process-functions=false
```

- Skip executing a YAML function when processing Atmos stack manifests

```
 $ atmos describe affected --skip=terraform.output
```
