- Generate Atlantis projects for the specified stacks only (comma-separated values).

```shell
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks <stack1>,<stack2>
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks tenant1-ue2-staging,tenant1-ue2-prod
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --stacks orgs/cp/tenant1/staging/us-east-2,tenant1-ue2-prod
```

- Generate Atlantis projects for the specified components only (comma-separated values)

```shell
 $ atmos atlantis generate repo-config --config-template <config_template> --project-template <project_template> --components <component1>,<component2>
```
- Generate Atlantis projects only for the Atmos components changed between two Git commits.

```shell
 $ atmos atlantis generate repo-config --affected-only
```

- Use to clone target

```shell
 $ atmos atlantis generate repo-config --affected-only --clone-target-ref
```

- Filesystem path to the already cloned target repository with which to compare the current branch

```shell
 $ atmos atlantis generate repo-config --affected-only --repo-path <path_to_already_cloned_repo>
```

- Git reference with which to compare the current branch

```shell
 $ atmos atlantis generate repo-config --affected-only --ref refs/heads/main
```

- Git commit SHA with which to compare the current branch

```shell
 $ atmos atlantis generate repo-config --affected-only --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073
```

- Print more detailed output when cloning and checking out the Git repository

```shell
 $  atmos atlantis generate repo-config --affected-only --verbose
```

- Path to PEM-encoded private key to clone private repos using SSH

```shell
 $ atmos atlantis generate repo-config --affected-only --ssh-key <path_to_ssh_key>
```

- Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block

```shell
 $  atmos atlantis generate repo-config --affected-only --ssh-key <path_to_ssh_key> --ssh-key-password <password>
```
