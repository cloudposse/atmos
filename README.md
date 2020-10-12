# variants

Universal CLI for DevOps and Cloud Automation.


## Introduction

`variants` is both a library and a command-line tool for provisioning, managing and orchestrating workflows across various toolchains. We use it extensively for automating cloud infrastructure and [Kubernetes](https://kubernetes.io/) clusters.

It includes workflows for dealing with:

  - Provisioning [Terraform](https://www.terraform.io/) projects
  - Deploying [helm](https://helm.sh/) [charts](https://helm.sh/docs/topics/charts/) to Kubernetes clusters using [helmfiles](https://github.com/roboll/helmfile)
  - Executing [helm](https://helm.sh/) commands on Kubernetes clusters
  - Provisioning [istio](https://istio.io/) on Kubernetes clusters using [istio operator](https://istio.io/latest/blog/2019/introducing-istio-operator/) and helmfile
  - Executing [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) commands on Kubernetes clusters
  - Executing [AWS SDK](https://aws.amazon.com/tools/) commands to orchestrate cloud infrastructure
  - Running [AWS CDK](https://aws.amazon.com/cdk/) constructs to define cloud resources
  - Executing commands for the [serverless](https://www.serverless.com/) framework
  - Executing shell commands
  - Combining various commands into workflows to execute many commands sequentially in just one step
  - ... and many more

In essence, it's a tool that orchestrates the other CLI tools in a consistent and self-explaining manner. It's a superset of all other tools and task runners (e.g. `make`, `terragrunt`, `terraform`, `aws` cli, `gcloud`, etc) and intended to be used to tie everything together so you can provide a simple CLI interface for your organization. 

Moreover, `variants` is not only a command-line interface for managing clouds and clusters. It provides many useful patterns and best practices, such as:

  - Enforces Terraform and helmfile projects' structure (so everybody knows where things are)
  - Provides separation of configuration and code (so the same code could be easily deployed to different regions, environments and stages)
  - It can be extended to include new features, commands, and workflows
  - The commands have a clean, consistent and easy to understand syntax
  - The CLI can be compiled into a binary and included in other tools and containers for DevOps, cloud automation and CI/CD
  - The CLI code is modular and self-documenting
## Recommended Layout

Our recommended filesystem layout looks like this:

~~~
└── cli/
    │   # Centralized configuration
    ├── config/
    │   │
    │   └── $env-$stage.yml  
    │    
    │   # Projects are broken down by tool
    ├── projects/
    │   │
    │   ├── terraform/   # root modules in here
    │   │   ├── vpc/
    │   │   ├── eks/
    │   │   ├── rds/
    │   │   ├── iam/
    │   │   ├── dns/
    │   │   └── sso/
    │   │
    │   └── helmfiles/  # helmfiles are organized by chart
    │       ├── cert-manager/helmfile.yaml
    │       └── external-dns/helmfile.yaml
    │   
    │   # Makefile for building the cli
    ├── Makefile
    │   
    │   # Docker image for shipping the cli and all dependencies
    └── Dockerfile (optional)

~~~

## CLI Structure

The CLI is built with [variant2](https://github.com/mumoshu/variant2) using [HCL syntax](https://www.terraform.io/docs/configuration/index.html).

`*.variant` files are combined like Terraform files. 

See `variant` docs for more information on [writing commands](https://github.com/mumoshu/variant2#writing-commands).

The CLI code consists of self-documenting [modules](modules) (separating the files into modules is done for cleanliness):

  - shell - `shell` commands and helpers for the other modules
  - terraform - `terraform` commands (`plan`, `apply`, `deploy`, `destroy`, `import`, etc.)
  - helm - `helm` commands (e.g. `list`)
  - helmfile - `helmfile` commands (`diff`, `apply`, `deploy`, `destroy`, `sync`, `lint`, etc.)
  - kubeconfig - commands to download and manage `kubeconfig` from EKS clusters
  - istio - commands to manage `istio` using `istio-operator` and `helmfile`
  - workflow - commands to construct and execute cloud automation workflows 


## Usage

The [example](example) folder contains a complete solution that shows how to:

  - Structure the terraform and helmfile projects
  - Configure the CLI top-level module [main.variant](example/cli/main.variant)
  - Add [configurations](example/config) (variables) for the terraform and helmfile projects (to provision them to different environments and stages)
  - Create a [Dockerfile](example/Dockerfile) with commands to build and include the CLI into the container
  - Combine many CLI commands into workflows and run the workflows to provision resources

In the example, we show how to create and provision (using the CLI) the following resources for three different environments/stages:

  - VPCs for `dev`, `staging` and `prod` stages in the `us-east-2` region (which we refer to as `ue2` environment)
  - EKS clusters in the `ue2` environment for `dev`, `staging` and `prod`
  - `ingress-nginx` helmfile to be deployed on all the EKS clusters
  - `istio` helmfile and workflow to deploy `istio` on the EKS clusters using `istio-operator`


### Configure the CLI

The CLI top-level module [main.variant](example/cli/main.variant) contains the global settings (options) for the CLI, including the location of the terraform projects,
helmfiles, and configurations.

It's configured for that particular example project, but can be changed to reflect the desired project structure.

In the example we have the following:

  - The terraform projects are in the [projects](example/projects) folder - and we set that global option in [main.variant](example/cli/main.variant)
  ```hcl
    option "project-dir" {
      default     = "./"
      description = "Terraform projects directory"
      type        = string
    }
  ```

  - The helmfiles are in the [projects/helmfiles](example/projects/helmfiles) folder - we set that global option in [main.variant](example/cli/main.variant)
  ```hcl
    option "helmfile-dir" {
      default     = "./helmfiles"
      description = "Helmfile projects directory"
      type        = string
    }
  ```

  - The configurations (Terraform and helmfile variables) are in the [config](example/config) folder - we set that global option in [main.variant](example/cli/main.variant)
  ```hcl
    option "config-dir" {
      default     = "../config"
      description = "Config directory"
      type        = string
    }
  ```

__NOTE:__ The container starts in the [projects](example/projects) directory (see [Dockerfile](example/Dockerfile)).
All paths are relative to the [projects](example/projects) directory, but can be easily changed (in the Dockerfile and in the CLI options) as needed.

[main.variant](example/cli/main.variant) also includes the `imports` statement that imports all the required modules from the `variants` repo.

__NOTE:__ For the example, we import all the CLI modules, but they could be included selectively depending on a particular usage.

```hcl
imports = [
  "git::https://git@github.com/cloudposse/variants@modules/shell?ref=master",
  "git::https://git@github.com/cloudposse/variants@modules/kubeconfig?ref=master",
  "git::https://git@github.com/cloudposse/variants@modules/terraform?ref=master",
  "git::https://git@github.com/cloudposse/variants@modules/helmfile?ref=master",
  "git::https://git@github.com/cloudposse/variants@modules/helm?ref=master",
  "git::https://git@github.com/cloudposse/variants@modules/workflow?ref=master",
  "git::https://git@github.com/cloudposse/variants@modules/istio?ref=master"
]
```

__NOTE:__ `imports` statement supports `https`, `http`, and `ssh` protocols.

__NOTE:__ The global options from [main.variant](example/cli/main.variant) are propagated to all the downloaded modules,
so they need to be specified only in one place - in the top-level module.

When we build the Docker image, all the modules from the `imports` statement are downloaded, combined with the top-level module [main.variant](example/cli/main.variant), 
and compiled into a binary, which then included in the container.


### Run the Example

To run the example, execute the following commands in a terminal:

  - `cd example`
  - `make all` - it will build the Docker image, build the `variants` CLI tool inside the image, and then start the container

Note that the name of the CLI executable is configurable.

In the [Dockerfile](example/Dockerfile) for the example, we've chosen the name `opsctl`, but it could be any name you want, for example
`ops`, `cli`, `ops-exe`, etc. The name of the CLI executable is configured using `ARG CLI_NAME=opsctl` in the Dockerfile.

After the container starts, run `opsctl help` to see the available commands and available flags.

__NOTE:__ We use Cloud Posse [geodesic](https://github.com/cloudposse/geodesic) as the base image for the container.
`geodesic` is the fastest way to get up and running with a rock solid, production grade cloud platform built entirely from Open Source technologies.


### Provision Terraform Project

To provision a Terraform project using the `opsctl` CLI, run the following commands in the container shell:

```bash
opsctl terraform plan eks --environment=ue2 --stage=dev
opsctl terraform apply eks --environment=ue2 --stage=dev
```

where:

  - `efs` is the Terraform project to provision (from the `projects` folder)
  - `--environment=ue2` is the environment to provision the project into (e.g. `ue2`, `uw2`). Note: the environments we are using here are abbreviations of AWS regions
  - `--stage=dev` is the stage/account (`prod`, `staging`, `dev`)

Short versions of the command-line arguments can be used:

```bash
opsctl terraform plan eks -e ue2 -s dev
opsctl terraform apply eks -e ue2 -s dev
```

To execute `plan` and `apply` in one step, use `terrafrom deploy` command:

```bash
opsctl terraform deploy eks -e ue2 -s dev
```

### Provision Helmfile Project

To provision a helmfile project using the `opsctl` CLI, run the following commands in the container shell:

```bash
opsctl helmfile diff ingress-nginx --environment=ue2 --stage=dev
opsctl helmfile apply ingress-nginx --environment=ue2 --stage=dev
```

where:

  - `ingress-nginx` is the helmfile project to provision (from the `projects/helmfiles` folder)
  - `--environment=ue2` is the environment to provision the project into (e.g. `ue2`, `uw2`). Note: the environments we are using here are abbreviations of AWS regions
  - `--stage=dev` is the stage/account (`prod`, `staging`, `dev`)

Short versions of the command-line arguments can be used:

```bash
opsctl helmfile diff ingress-nginx -e ue2 -s dev
opsctl helmfile apply ingress-nginx -e ue2 -s dev
```

To execute `diff` and `apply` in one step, use `helmfile deploy` command:

```bash
opsctl helmfile deploy ingress-nginx -e ue2 -s dev
```

### Deploy istio

To deploy `istio` into a Kubernetes cluster, run the following commands:

```bash
opsctl istioctl operator-init -e ue2 -s dev
opsctl helmfile deploy istio -e ue2 -s dev
```

This will install the `istio` operator first, then provision `istio` using the helmfile.
 

### Run Workflows

Workflows are a way of combining multiple commands into one executable unit of work.

In the CLI, workflows can be defined using two different methods:

  - In the configuration file for an environment/stage (see [workflows in ue2-dev.yaml](example/config/ue2-dev.yaml) for an example)
  - In a separate file (see [workflows-all.yaml](example/config/workflows-all.yaml) and [workflows-istio.yaml](example/config/workflows-istio.yaml))

In the first case, we define workflows in the configuration file for the environment and stage (which we specify on the command line).
To execute the workflows from [workflows in ue2-dev.yaml](example/config/ue2-dev.yaml), run the following commands:

```bash
opsctl workflow deploy-all -e ue2 -s dev
opsctl workflow istio-init -e ue2 -s dev
```

Note that workflows defined in the environment/stage config files can be executed only for the particular environment and stage.
It's not possible to provision resources for multiple environments and stages this way.

In the second case (defining workflows in a separate file), a single workflow can be created to provision resources into different environments/stages.
The environments/stages for the workflow steps can be specified in the workflow config.

For example, to run `terraform plan` and `helmfile diff` on all terraform and helmfile projects in the example, execute the following command:

```bash
opsctl workflow plan-all -f workflows-all
```

where the command-line option `-f` (`--file` for long version) instructs the `opsctl` CLI to look for the `plan-all` workflow in the file [workflows-all](example/config/workflows-all.yaml).

As we can see, in multi-environment workflows, each workflow job specifies the environment and stage it's operating on:

```yaml
workflows:
  plan-all:
    description: Run 'terraform plan' and 'helmfile diff' on all projects for all environments/stages
    steps:
      - job: terraform plan vpc
        environment: ue2
        stage: dev
      - job: terraform plan eks
        environment: ue2
        stage: dev
      - job: helmfile diff ingress-nginx
        environment: ue2
        stage: dev
      - job: terraform plan vpc
        environment: ue2
        stage: staging
      - job: terraform plan eks
        environment: ue2
        stage: staging
```

You can also define a workflow in a separate file without specifying the environment and stage in the workflow's job config.
In this case, the environment and stage need to be provided on the command line.

For example, to run the `deploy-all` workflow from the [workflows-all](example/config/workflows-all.yaml) file for the environment `ue2` and stage`dev`,
execute the following command:

```bash
opsctl workflow deploy-all -f workflows-all -e ue2 -s dev
```
