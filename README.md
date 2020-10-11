# variants

Universal CLI for DevOps and Cloud Automation.


## Introduction

`variants` is a command-line tool for provisioning, managing and orchestrating cloud infrastructure and [Kubernetes](https://kubernetes.io/) clusters.

It includes commands for:

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

In essence, it's a tool that orchestrates the other CLI tools using a clean and consistent syntax.

Moreover, `variants` is not only a command-line interface for managing clouds and clusters. It provides many useful patterns and best practices, such as:

  - Enforces Terraform and helmfile projects' structure (so everybody knows where things are)
  - Provides separation of configuration and code (so the same code could be easily deployed to different regions, environments and stages)
  - It can be extended to include new features, commands, and workflows
  - The commands have a consistent and easy to understand syntax
  - The CLI can be compiled into a binary and included in other tools and containers for DevOps, cloud automation and CI/CD
  - The CLI code is modular and self-documenting


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

Run `opsctl help` to see the available commands and available flags.

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

### Docker Access

The geodesic `infrastructure` image builds the CLI through the normal build process. Build and run the image as normal to access the `opsctl` cli.


