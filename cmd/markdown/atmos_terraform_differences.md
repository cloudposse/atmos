# Additions and Differences from Native Terraform

Atmos enhances Terraform with additional automation and safety features. Below are the key differences:

## Automatic Initialization

**Before executing terraform commands, Atmos runs `terraform init` automatically.**

- Skip initialization with `--skip-init` flag when your project is already initialized:
  ```shell
  atmos terraform <command> <component> -s <stack> --skip-init
  ```

## Deploy Command

The `atmos terraform deploy` command provides automated deployment:

- Executes `terraform apply -auto-approve` (bypasses manual approval)
- Supports `--deploy-run-init=true|false` to control initialization
- Use `--from-plan` flag to apply a previously generated planfile:
  ```shell
  atmos terraform deploy <component> -s <stack> --from-plan
  ```

## Plan Command Enhancements

### Working with Planfiles

- **Apply command with planfiles:**
  ```shell
  # Generate plan
  atmos terraform plan <component> -s <stack> -out=<FILE>

  # Apply the plan
  atmos terraform apply <component> -s <stack> --planfile <FILE>
  ```

- **Skip planfile generation:**
  ```shell
  atmos terraform plan <component> -s <stack> --skip-planfile=true
  ```
  Use this when working with Terraform Cloud, which doesn't support local planfiles.

## Clean Command

The `atmos terraform clean` command removes temporary files and state:

- Deletes `.terraform` folder, `.terraform.lock.hcl` lock file, and generated planfiles/varfiles
- Use `--skip-lock-file` to preserve the lock file
- **Deletes local state files** (including `terraform.tfstate.d`)
- Use `--force` to bypass confirmation prompt

⚠️ **Warning:** This command can lead to permanent state loss if not using remote backends.

## Workspace Management

The `atmos terraform workspace` command automates workspace setup:

1. Runs `terraform init -reconfigure`
2. Runs `terraform workspace select`
3. Creates workspace with `terraform workspace new` if needed

## Import Command

The `atmos terraform import` command automatically sets AWS region:

- Searches for `region` variable in component configuration
- Sets `AWS_REGION=<region>` environment variable before import

## Backend Generation

Generate backend configurations for Terraform remote state:

- **Single component:**
  ```shell
  atmos terraform generate backend <component> -s <stack>
  ```

- **All components:**
  ```shell
  atmos terraform generate backends
  ```

## Varfile Generation

Generate variable files for Terraform:

- **Single component:**
  ```shell
  atmos terraform generate varfile <component> -s <stack>
  ```

- **All components:**
  ```shell
  atmos terraform generate varfiles
  ```

## Plan Diff

Compare two Terraform plans to see differences:

```shell
atmos terraform plan-diff --orig <original-plan> --new <new-plan>
```

If `--new` is not provided, generates a new plan with current configuration.

## Shell Mode

Launch an interactive shell with Atmos environment configured:

```shell
atmos terraform shell <component> -s <stack>
```

Execute native terraform commands inside the configured shell environment.

## Passing Native Terraform Flags

Use double-dash `--` to pass additional flags to terraform:

```shell
atmos terraform plan <component> -s <stack> -- -refresh=false
atmos terraform apply <component> -s <stack> -- -lock=false
```
